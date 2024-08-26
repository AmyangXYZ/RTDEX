package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"pntaas/packet"
	"sync"
	"sync/atomic"
	"time"
)

type SessionManager struct {
	Sessions sync.Map
}

type pktCallback struct {
	Pkt *packet.PNTaaSPacket
	Cb  func()
}

type Session struct {
	ID                     uint32       `json:"id"`
	Type                   string       `json:"type"`
	Lifetime               int          `json:"lifetime"`
	Location               [3]float64   `json:"location"`
	RemoteAddr             *net.UDPAddr `json:"addr"`
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

func NewSessionManager() *SessionManager {
	mgr := &SessionManager{}
	go mgr.housekeeping()
	go mgr.WatchSlotSignal()
	return mgr
}

func (m *SessionManager) housekeeping() {
	ticker := time.NewTicker(HouseKeepingInterval)
	for range ticker.C {
		m.Sessions.Range(func(id, v interface{}) bool {
			session := v.(*Session)
			if session.Lifetime == 0 {
				session.End()
				m.Sessions.Delete(id)
			}
			return true
		})
	}
}

func (m *SessionManager) CreateSession(id uint32, type_ packet.DeviceType, remoteAddr *net.UDPAddr) *Session {
	session := NewSession(id, type_, remoteAddr)
	m.Sessions.Store(id, session)
	return session
}

func (m *SessionManager) GetSession(id uint32) *Session {
	session, exists := m.Sessions.Load(id)
	if !exists {
		return nil
	}
	return session.(*Session)
}

func (m *SessionManager) RemoveSession(id uint32) {
	session, _ := m.Sessions.LoadAndDelete(id)
	session.(*Session).End()
}

func (m *SessionManager) WatchSlotSignal() {
	for slot := range slotMgr.SlotIncrementSignal {
		m.Sessions.Range(func(_, v interface{}) bool {
			session := v.(*Session)
			session.SlotIncrement(slot)
			return true
		})
	}
}

func NewSession(id uint32, type_ packet.DeviceType, remoteAddr *net.UDPAddr) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	t := packet.DeviceType_name[int32(type_)]
	logger := log.New(log.Writer(), fmt.Sprintf("[Session %d] ", id), 0)
	return &Session{
		ID:                     id,
		Type:                   t,
		Lifetime:               InitialLifetime,
		logger:                 logger,
		RemoteAddr:             remoteAddr,
		outQueueHighPriority:   make(chan *packet.PNTaaSPacket, PktQueueSize),
		outQueueMediumPriority: make(chan *packet.PNTaaSPacket, PktQueueSize),
		outQueueLowPriority:    make(chan *packet.PNTaaSPacket, PktQueueSize),
		slotIncrementSignal:    make(chan int),
		ctx:                    ctx,
		cancel:                 cancel,
	}
}

func (s *Session) UpdateRemoteAddr(remoteAddr *net.UDPAddr) {
	s.RemoteAddr = remoteAddr
	s.logger.Printf("Remote address updated to %s", s.RemoteAddr.String())
}

func (s *Session) ResetLifetime() {
	s.Lifetime = InitialLifetime
}

func (s *Session) SlotIncrement(slot int) {
	select {
	case s.slotIncrementSignal <- slot:
	default:
	}
}

func (s *Session) End() {
	s.logger.Println("Ending session")
	s.cancel()
}

func (s *Session) lifetimeTimer() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Println("Stop lifetime timer")
			return
		case <-ticker.C:
			if s.Lifetime > 0 {
				s.Lifetime--
			} else {
				s.logger.Println("Lifetime expired")
			}
		}
	}
}

func (s *Session) Start() {
	go s.lifetimeTimer()
	s.processQueuesTimeAware()
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
	if err := server.SendToRemote(pkt, s.RemoteAddr); err != nil {
		s.logger.Println(err)
	}
}

func (s *Session) HandlePacket(pkt *packet.PNTaaSPacket) bool {
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
		expiry := time.Now().Add(time.Duration(freshness) * time.Second)
		cache.Set(name, &CachedData{Name: name, Owner: pkt.Header.SourceId, data: []byte{}, Expiry: expiry})
	case packet.PacketType_DATA_CONTENT:
		name := pkt.GetDataContent().Name
		data := pkt.GetDataContent().Data
		checksum := pkt.GetDataContent().Checksum
		if cachedData := cache.Get(name); cachedData != nil {
			if crc32.ChecksumIEEE(data) == checksum {
				cachedData.data = data
				cachedData.Size = len(data)
				cachedData.Checksum = checksum
				cache.Set(name, cachedData)
				s.logger.Printf("Received data content for %s, checksum: %X, size: %d", name, checksum, len(data))
			} else {
				s.logger.Printf("Checksum mismatch for %s, %X:%X, %d", name, crc32.ChecksumIEEE(data), checksum, len(data))
				s.sendErrorMessage(pkt.Header.PacketUid, packet.ErrorCode_DATA_CHECK_SUM_FAILED)
			}
		} else {
			s.logger.Printf("Data not registered for %s", name)
			s.sendErrorMessage(pkt.Header.PacketUid, packet.ErrorCode_DATA_NOT_FOUND)
		}

	case packet.PacketType_DATA_INTEREST:
		name := pkt.GetDataInterest().Name
		if cachedData := cache.Get(name); cachedData == nil {
			s.logger.Printf("Data not found for %s", name)
			s.sendErrorMessage(pkt.Header.PacketUid, packet.ErrorCode_DATA_NOT_FOUND)
		} else if cachedData.Size == 0 || cachedData.Checksum == 0 {
			s.logger.Printf("Data not ready for %s", name)
			s.sendErrorMessage(pkt.Header.PacketUid, packet.ErrorCode_DATA_NOT_READY)
		} else {
			cachedData.Downloads++
			s.sendDataContent(name, cachedData)
		}
	default:
		return false
	}
	return true
}

func (s *Session) sendAck(uid uint32, tx_timestamp uint64) {
	s.outQueueHighPriority <- packet.CreateAcknowledgementPacket(server.ID, s.ID, uid, tx_timestamp)
}

func (s *Session) sendErrorMessage(uid uint32, code packet.ErrorCode) {
	s.outQueueHighPriority <- packet.CreateErrorMessagePacket(server.ID, s.ID, uid, code)
}

func (s *Session) sendJoinResponse() {
	pkt := packet.CreateJoinResponsePacket(server.ID, s.ID, 2024)
	s.outQueueHighPriority <- pkt
	cb := &pktCallback{
		Pkt: pkt,
		Cb:  s.ackTimeoutCallback(pkt),
	}
	s.pktAckCallbacks.Store(pkt.Header.PacketUid, cb)
}

func (s *Session) sendDataContent(name string, cachedData *CachedData) {
	pkt := packet.CreateDataContentPacket(server.ID, s.ID, name, cachedData.Checksum, cachedData.data)
	s.outQueueHighPriority <- pkt
	ackCb := &pktCallback{
		Pkt: pkt,
		Cb:  s.ackTimeoutCallback(pkt),
	}
	s.pktAckCallbacks.Store(pkt.Header.PacketUid, ackCb)
	errCb := &pktCallback{
		Pkt: pkt,
		Cb: func() {
			s.outQueueHighPriority <- pkt // retransmit
		},
	}
	s.pktErrCallbacks.Store(pkt.Header.PacketUid, errCb)
}

func (s *Session) ackTimeoutCallback(pkt *packet.PNTaaSPacket) func() {
	retries := 0
	var timer *time.Timer
	cb := func() {
		if _, ok := s.pktAckCallbacks.Load(pkt.Header.PacketUid); ok {
			if retries < AckMaxRetries {
				retries++
				s.logger.Printf("Packet 0x%X: ACK timeout or error, retrying (%d/%d)", pkt.Header.PacketUid, retries, AckMaxRetries)
				s.outQueueHighPriority <- pkt // retransmit
				timer.Reset(AckTimeout)
			} else {
				s.logger.Printf("Packet 0x%X: Max retries reached, giving up", pkt.Header.PacketUid)
				s.pktAckCallbacks.Delete(pkt.Header.PacketUid)
			}
		}
	}

	timer = time.AfterFunc(AckTimeout, cb)

	return func() {
		if timer != nil {
			timer.Stop()
		}
		cb()
	}
}
