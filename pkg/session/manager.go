package session

import (
	"net"
	"sync"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/core"
)

type SessionManager struct {
	engine   core.Engine
	Sessions sync.Map
}

func NewSessionManager(engine core.Engine) core.SessionManager {
	return &SessionManager{
		engine: engine,
	}
}

func (m *SessionManager) Start() {
	go m.Housekeeping()
	go m.WatchSlotSignal()

	<-m.engine.Ctx().Done()
	m.RemoveAllSessions()
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

func (m *SessionManager) CreateSession(id uint32, remoteAddr *net.UDPAddr) core.Session {
	session := NewSession(m.engine, id, remoteAddr)
	m.Sessions.Store(id, session)
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
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		m.Sessions.Range(func(id, v interface{}) bool {
			session := v.(core.Session)
			if session.Lifetime() == 0 {
				session.Stop()
				m.Sessions.Delete(id)
			}
			return true
		})
	}
}
