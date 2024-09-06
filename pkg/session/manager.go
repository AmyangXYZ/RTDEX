package session

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/AmyangXYZ/rtdex/internal/utils"
	"github.com/AmyangXYZ/rtdex/pkg/core"
)

type SessionManager struct {
	engine      core.Engine
	Sessions    sync.Map
	expiryQueue *utils.ExpiryPriorityQueue
	logger      *log.Logger
}

func NewSessionManager(engine core.Engine) core.SessionManager {
	return &SessionManager{
		engine:      engine,
		expiryQueue: utils.NewExpiryPriorityQueue(),
		logger:      log.New(log.Writer(), "[SessionManager] ", 0),
	}
}

func (m *SessionManager) Start() {
	go m.Housekeeping()
	go m.WatchSlotSignal()

	<-m.engine.Ctx().Done()
	m.logger.Println("Stop sessions")
	m.RemoveAllSessions()
	m.expiryQueue = utils.NewExpiryPriorityQueue()
}

func (m *SessionManager) WatchSlotSignal() {
	for slot := range m.engine.SlotManager().SlotSignal() {
		m.Sessions.Range(func(_, v interface{}) bool {
			session := v.(*Session)
			session.SlotIncrement(slot)
			return true
		})
	}
}

func (m *SessionManager) CreateSession(id uint32, namespace string, remoteAddr *net.UDPAddr) core.Session {
	session := NewSession(m.engine, id, namespace, remoteAddr)
	m.Sessions.Store(id, session)
	m.expiryQueue.Push(&utils.ExpiringItem{
		Value:    session,
		ExpireAt: session.Expiry(),
	})
	return session
}

func (m *SessionManager) GetSession(id uint32) core.Session {
	session, exists := m.Sessions.Load(id)
	if !exists {
		return nil
	}
	return session.(core.Session)
}

func (m *SessionManager) GetAllSessions() []core.Session {
	sessions := []core.Session{}
	m.Sessions.Range(func(_, v interface{}) bool {
		sessions = append(sessions, v.(core.Session))
		return true
	})
	return sessions
}

func (m *SessionManager) RemoveSession(id uint32) {
	session, _ := m.Sessions.LoadAndDelete(id)
	session.(core.Session).Stop()
}

func (m *SessionManager) RemoveAllSessions() {
	m.Sessions.Range(func(_, v interface{}) bool {
		session := v.(core.Session)
		session.Stop()
		m.Sessions.Delete(session.ID())
		return true
	})
}

func (m *SessionManager) Housekeeping() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		for {
			item := m.expiryQueue.Peek()
			if item == nil || item.ExpireAt.After(time.Now()) {
				break
			}
			m.expiryQueue.Pop()
			session := item.Value.(core.Session)
			m.logger.Printf("Removed expired session %d\n", session.ID())
			m.RemoveSession(session.ID())
		}
	}
}
