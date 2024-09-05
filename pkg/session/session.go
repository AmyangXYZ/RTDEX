package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
)

type pktCallback struct {
	Pkt *packet.RTDEXPacket
	Cb  func()
}

type Session struct {
	engine                 core.Engine
	id                     uint32
	namespace              string
	lifetime               int
	remoteAddr             *net.UDPAddr
	logger                 *log.Logger
	pktAckCallbacks        sync.Map
	pktErrCallbacks        sync.Map
	outQueueHighPriority   chan *packet.RTDEXPacket
	outQueueMediumPriority chan *packet.RTDEXPacket
	outQueueLowPriority    chan *packet.RTDEXPacket
	slotIncrementSignal    chan int
	isSending              atomic.Bool
	ctx                    context.Context
	cancel                 context.CancelFunc
}

func NewSession(engine core.Engine, id uint32, namespace string, remoteAddr *net.UDPAddr) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.New(log.Writer(), fmt.Sprintf("[Session %d] ", id), 0)
	return &Session{
		engine:                 engine,
		id:                     id,
		namespace:              namespace,
		lifetime:               engine.Config().SessionLifetime,
		logger:                 logger,
		remoteAddr:             remoteAddr,
		outQueueHighPriority:   make(chan *packet.RTDEXPacket, engine.Config().PktQueueSize),
		outQueueMediumPriority: make(chan *packet.RTDEXPacket, engine.Config().PktQueueSize),
		outQueueLowPriority:    make(chan *packet.RTDEXPacket, engine.Config().PktQueueSize),
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

func (s *Session) Namespace() string {
	return s.namespace
}

func (s *Session) Lifetime() int {
	return s.lifetime
}

func (s *Session) RemoteAddr() string {
	return s.remoteAddr.String()
}

func (s *Session) ResetLifetime() {
	s.lifetime = s.engine.Config().SessionLifetime
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
	s.logger.Println("Stop session")
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

func (s *Session) tryGetPacket(queue chan *packet.RTDEXPacket) *packet.RTDEXPacket {
	select {
	case pkt := <-queue:
		return pkt
	default:
		return nil
	}
}

func (s *Session) sendPacket(pkt *packet.RTDEXPacket) {
	s.logger.Printf("Send %s-0x%X\n", pkt.GetHeader().PacketType, pkt.GetHeader().PacketUid)
	if err := s.engine.Server().Send(pkt, s.remoteAddr); err != nil {
		s.logger.Println(err)
	}
}

func (s *Session) HandlePacket(pkt *packet.RTDEXPacket) {
	s.logger.Printf("Received %s-0x%X\n", pkt.GetHeader().PacketType, pkt.GetHeader().PacketUid)
	if pkt.GetHeader().Priority > packet.Priority_LOW && pkt.GetHeader().PacketType != packet.PacketType_ACKNOWLEDGEMENT {
		s.sendAck(pkt.GetHeader().PacketUid, pkt.GetHeader().Timestamp)
	}

	s.ResetLifetime()

	switch pkt.GetHeader().PacketType {
	case packet.PacketType_ACKNOWLEDGEMENT:
		if callback, _ := s.pktAckCallbacks.LoadAndDelete(pkt.GetHeader().PacketUid); callback != nil {
			callback.(*pktCallback).Cb()
		} else {
			s.logger.Printf("ACK received for unknown packet 0x%X", pkt.GetHeader().PacketUid)
		}
	case packet.PacketType_ERROR_MESSAGE:
		if callback, _ := s.pktErrCallbacks.LoadAndDelete(pkt.GetHeader().PacketUid); callback != nil {
			callback.(*pktCallback).Cb()
		} else {
			s.logger.Printf("Error message received for unknown packet 0x%X", pkt.GetHeader().PacketUid)
		}
	case packet.PacketType_JOIN_REQUEST:
		s.sendJoinResponse()
	case packet.PacketType_DATA_REGISTER:
		name := pkt.GetDataRegister().Name
		size := pkt.GetDataRegister().Size
		freshness := pkt.GetDataRegister().Freshness
		rootChecksum := pkt.GetDataRegister().Checksum
		numChunks := pkt.GetDataRegister().NumChunks
		s.logger.Printf("Received data register request for %s, size: %d, freshness: %d, root checksum: %X, num chunks: %d", name, size, freshness, rootChecksum, numChunks)
		s.engine.Cache().Set(
			name,
			&core.CacheItem{
				Name:           name,
				Size:           int(size),
				Checksum:       rootChecksum,
				NumChunks:      int(numChunks),
				Chunks:         make(map[int][]byte),
				ChunkChecksums: make(map[int]uint32),
				Expiry:         time.Now().Add(time.Duration(freshness) * time.Second),
			},
		)
	case packet.PacketType_DATA_CONTENT:
		name := pkt.GetDataContent().Name
		chunkIndex := int(pkt.GetDataContent().ChunkIndex)
		chunkChecksum := pkt.GetDataContent().Checksum
		data := pkt.GetDataContent().Data

		if cacheItem := s.engine.Cache().Get(name); cacheItem != nil {
			if crc32.ChecksumIEEE(data) == chunkChecksum {
				cacheItem.Chunks[chunkIndex] = data
				cacheItem.ChunkChecksums[chunkIndex] = chunkChecksum
				s.logger.Printf("Received chunk %d for %s, checksum: %X", chunkIndex, name, chunkChecksum)

				if len(cacheItem.Chunks) == cacheItem.NumChunks {
					// All chunks received, verify root checksum
					allChecksums := make([]byte, 4*cacheItem.NumChunks)
					for i := 0; i < cacheItem.NumChunks; i++ {
						binary.LittleEndian.PutUint32(allChecksums[i*4:], cacheItem.ChunkChecksums[i])
					}
					rootChecksum := crc32.ChecksumIEEE(allChecksums)
					if rootChecksum == cacheItem.Checksum {
						s.logger.Printf("All chunks received and verified for %s, root checksum: %X", name, rootChecksum)
					} else {
						s.logger.Printf("Root checksum mismatch for %s", name)
						s.sendErrorMessage(pkt.GetHeader().PacketUid, packet.ErrorCode_DATA_CHECK_SUM_FAILED)
					}
				}
				s.engine.Cache().Set(name, cacheItem)
			} else {
				s.logger.Printf("Chunk checksum mismatch for %s, chunk %d", name, chunkIndex)
				s.sendErrorMessage(pkt.GetHeader().PacketUid, packet.ErrorCode_DATA_CHECK_SUM_FAILED)
			}
		} else {
			s.logger.Printf("Data not registered for %s", name)
			s.sendErrorMessage(pkt.GetHeader().PacketUid, packet.ErrorCode_DATA_NOT_FOUND)
		}
	case packet.PacketType_DATA_INTEREST:
		name := pkt.GetDataInterest().Name
		if cacheItem := s.engine.Cache().Get(name); cacheItem == nil {
			s.logger.Printf("Data not found for %s", name)
			s.sendErrorMessage(pkt.GetHeader().PacketUid, packet.ErrorCode_DATA_NOT_FOUND)
		} else if cacheItem.NumChunks > len(cacheItem.Chunks) {
			s.logger.Printf("Data not ready for %s", name)
			s.sendErrorMessage(pkt.GetHeader().PacketUid, packet.ErrorCode_DATA_NOT_READY)
		} else {
			s.sendDataInterestResponse(name, cacheItem.Checksum, uint32(cacheItem.NumChunks))
			s.sendDataContent(name, cacheItem)
		}
	default:
		s.logger.Printf("Received unknown packet type: %s", pkt.GetHeader().PacketType)
	}
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
	s.pktAckCallbacks.Store(pkt.GetHeader().PacketUid, cb)
}

func (s *Session) sendDataInterestResponse(name string, checksum uint32, numChunks uint32) {
	pkt := packet.CreateDataInterestResponsePacket(s.engine.Server().ID(), s.id, name, checksum, numChunks)
	s.outQueueHighPriority <- pkt
	cb := &pktCallback{
		Pkt: pkt,
		Cb:  s.ackTimeoutCallback(pkt),
	}
	s.pktAckCallbacks.Store(pkt.GetHeader().PacketUid, cb)
}

func (s *Session) sendDataContent(name string, cacheItem *core.CacheItem) {
	for i := 0; i < cacheItem.NumChunks; i++ {
		pkt := packet.CreateDataContentPacket(s.engine.Server().ID(), s.id, name, uint32(i), cacheItem.ChunkChecksums[i], cacheItem.Chunks[i])
		s.outQueueHighPriority <- pkt
		ackCb := &pktCallback{
			Pkt: pkt,
			Cb:  s.ackTimeoutCallback(pkt),
		}
		s.pktAckCallbacks.Store(pkt.GetHeader().PacketUid, ackCb)
		errCb := &pktCallback{
			Pkt: pkt,
			Cb: func() {
				s.outQueueHighPriority <- pkt // retransmit
			},
		}
		s.pktErrCallbacks.Store(pkt.GetHeader().PacketUid, errCb)
	}
}

func (s *Session) ackTimeoutCallback(pkt *packet.RTDEXPacket) func() {
	retries := 0
	var timer *time.Timer
	cb := func() {
		if _, ok := s.pktAckCallbacks.Load(pkt.GetHeader().PacketUid); ok {
			if retries < s.engine.Config().AckMaxRetries {
				retries++
				s.logger.Printf("Packet 0x%X: ACK timeout or error, retrying (%d/%d)", pkt.GetHeader().PacketUid, retries, s.engine.Config().AckMaxRetries)
				s.outQueueHighPriority <- pkt // retransmit
				timer.Reset(s.engine.Config().AckTimeout)
			} else {
				s.logger.Printf("Packet 0x%X: Max retries reached, giving up", pkt.GetHeader().PacketUid)
				s.pktAckCallbacks.Delete(pkt.GetHeader().PacketUid)
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
