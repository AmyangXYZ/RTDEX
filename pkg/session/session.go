package session

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
)

type pktCallback struct {
	Pkt *packet.PNTaaSPacket
	Cb  func()
}

type Session struct {
	engine                 core.Engine
	id                     uint32
	lifetime               int
	remoteAddr             *net.UDPAddr
	logger                 *log.Logger
	pktAckCallbacks        sync.Map
	pktErrCallbacks        sync.Map
	outQueueHighPriority   chan *packet.PNTaaSPacket
	outQueueMediumPriority chan *packet.PNTaaSPacket
	outQueueLowPriority    chan *packet.PNTaaSPacket
	slotIncrementSignal    chan int
	isSending              atomic.Bool
	ctx                    context.Context
	cancel                 context.CancelFunc
}

func NewSession(engine core.Engine, id uint32, remoteAddr *net.UDPAddr) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.New(log.Writer(), fmt.Sprintf("[Session %d] ", id), 0)
	return &Session{
		engine:                 engine,
		id:                     id,
		lifetime:               engine.Config().InitialLifetime,
		logger:                 logger,
		remoteAddr:             remoteAddr,
		outQueueHighPriority:   make(chan *packet.PNTaaSPacket, engine.Config().PktQueueSize),
		outQueueMediumPriority: make(chan *packet.PNTaaSPacket, engine.Config().PktQueueSize),
		outQueueLowPriority:    make(chan *packet.PNTaaSPacket, engine.Config().PktQueueSize),
		slotIncrementSignal:    make(chan int),
		ctx:                    ctx,
		cancel:                 cancel,
	}
}

func (s *Session) UpdateRemoteAddr(remoteAddr *net.UDPAddr) {
	s.remoteAddr = remoteAddr
	s.logger.Printf("Remote address updated to %s", s.remoteAddr.String())
}

func (s *Session) ID() uint32 {
	return s.id
}

func (s *Session) Lifetime() int {
	return s.lifetime
}

func (s *Session) RemoteAddr() string {
	return s.remoteAddr.String()
}

func (s *Session) ResetLifetime() {
	s.lifetime = s.engine.Config().InitialLifetime
}

func (s *Session) SlotIncrement(slot int) {
	select {
	case s.slotIncrementSignal <- slot:
	default:
	}
}

func (s *Session) lifetimeTimer() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("Stop lifetime timer")
			return
		case <-ticker.C:
			if s.lifetime > 0 {
				s.lifetime--
			} else {
				s.logger.Println("Lifetime expired")
			}
		}
	}
}

func (s *Session) Start() {
	go s.lifetimeTimer()
	go func() {

	}()
	s.processQueuesTimeAware()
}

func (s *Session) Stop() {
	s.logger.Println("Stopping session")
	s.cancel()
}

func (s *Session) processQueuesTimeAware() {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("Stop queue processing")
			return
		case <-s.slotIncrementSignal:
			// Always process high-priority packets immediately
			if pkt := s.tryGetPacket(s.outQueueHighPriority); pkt != nil {
				s.sendPacket(pkt) // Synchronous send for high-priority
				continue
			}

			// Only process lower priority if not already processing
			if !s.isSending.Load() {
				s.isSending.Store(true)
				go func() {
					defer s.isSending.Store(false)
					if pkt := s.tryGetPacket(s.outQueueMediumPriority); pkt != nil {
						s.sendPacket(pkt)
					} else if pkt := s.tryGetPacket(s.outQueueLowPriority); pkt != nil {
						s.sendPacket(pkt)
					}
				}()
			}
		}
	}
}

func (s *Session) tryGetPacket(queue chan *packet.PNTaaSPacket) *packet.PNTaaSPacket {
	select {
	case pkt := <-queue:
		return pkt
	default:
		return nil
	}
}

func (s *Session) sendPacket(pkt *packet.PNTaaSPacket) {
	s.logger.Printf("Send %s-0x%X\n", pkt.Header.PacketType, pkt.Header.PacketUid)
	if err := s.engine.Server().Send(pkt, s.remoteAddr); err != nil {
		s.logger.Println(err)
	}
}

func (s *Session) HandlePacket(pkt *packet.PNTaaSPacket) {
	s.logger.Printf("Received %s-0x%X\n", pkt.Header.PacketType, pkt.Header.PacketUid)
	if pkt.Header.Priority > packet.Priority_LOW && pkt.Header.PacketType != packet.PacketType_ACKNOWLEDGEMENT {
		s.sendAck(pkt.Header.PacketUid, pkt.Header.Timestamp)
	}

	s.ResetLifetime()

	switch pkt.Header.PacketType {
	case packet.PacketType_ACKNOWLEDGEMENT:
		if callback, _ := s.pktAckCallbacks.LoadAndDelete(pkt.Header.PacketUid); callback != nil {
			callback.(*pktCallback).Cb()
		} else {
			s.logger.Printf("ACK received for unknown packet 0x%X", pkt.Header.PacketUid)
		}
	case packet.PacketType_ERROR_MESSAGE:
		if callback, _ := s.pktErrCallbacks.LoadAndDelete(pkt.Header.PacketUid); callback != nil {
			callback.(*pktCallback).Cb()
		} else {
			s.logger.Printf("Error message received for unknown packet 0x%X", pkt.Header.PacketUid)
		}
	case packet.PacketType_JOIN_REQUEST:
		s.sendJoinResponse()
	case packet.PacketType_DATA_REGISTER:
		name := pkt.GetDataRegister().Name
		size := pkt.GetDataRegister().Size
		freshness := pkt.GetDataRegister().Freshness
		s.logger.Printf("Received data register request for %s, size: %d, freshness: %d", name, size, freshness)
		s.engine.Cache().Set(name, []byte{}, time.Duration(freshness)*time.Second)
	case packet.PacketType_DATA_CONTENT:

	case packet.PacketType_DATA_INTEREST:

	default:
		return
	}
	return
}

func (s *Session) sendAck(uid uint32, tx_timestamp uint64) {
	s.outQueueHighPriority <- packet.CreateAcknowledgementPacket(s.engine.Server().ID(), s.id, uid, tx_timestamp)
}

func (s *Session) sendErrorMessage(uid uint32, code packet.ErrorCode) {
	s.outQueueHighPriority <- packet.CreateErrorMessagePacket(s.engine.Server().ID(), s.id, uid, code)
}

func (s *Session) sendJoinResponse() {
	pkt := packet.CreateJoinResponsePacket(s.engine.Server().ID(), s.id, 2024)
	s.outQueueHighPriority <- pkt
	cb := &pktCallback{
		Pkt: pkt,
		Cb:  s.ackTimeoutCallback(pkt),
	}
	s.pktAckCallbacks.Store(pkt.Header.PacketUid, cb)
}

// func (s *Session) sendDataContent(name string, cachedData *CachedData) {
// 	pkt := packet.CreateDataContentPacket(server.ID, s.ID, name, cachedData.Checksum, cachedData.data)
// 	s.outQueueHighPriority <- pkt
// 	ackCb := &pktCallback{
// 		Pkt: pkt,
// 		Cb:  s.ackTimeoutCallback(pkt),
// 	}
// 	s.pktAckCallbacks.Store(pkt.Header.PacketUid, ackCb)
// 	errCb := &pktCallback{
// 		Pkt: pkt,
// 		Cb: func() {
// 			s.outQueueHighPriority <- pkt // retransmit
// 		},
// 	}
// 	s.pktErrCallbacks.Store(pkt.Header.PacketUid, errCb)
// }

func (s *Session) ackTimeoutCallback(pkt *packet.PNTaaSPacket) func() {
	retries := 0
	var timer *time.Timer
	cb := func() {
		if _, ok := s.pktAckCallbacks.Load(pkt.Header.PacketUid); ok {
			if retries < s.engine.Config().AckMaxRetries {
				retries++
				s.logger.Printf("Packet 0x%X: ACK timeout or error, retrying (%d/%d)", pkt.Header.PacketUid, retries, s.engine.Config().AckMaxRetries)
				s.outQueueHighPriority <- pkt // retransmit
				timer.Reset(s.engine.Config().AckTimeout)
			} else {
				s.logger.Printf("Packet 0x%X: Max retries reached, giving up", pkt.Header.PacketUid)
				s.pktAckCallbacks.Delete(pkt.Header.PacketUid)
			}
		}
	}

	timer = time.AfterFunc(s.engine.Config().AckTimeout, cb)

	return func() {
		if timer != nil {
			timer.Stop()
		}
		cb()
	}
}
