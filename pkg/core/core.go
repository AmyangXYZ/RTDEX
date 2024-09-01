package core

import (
	"context"
	"net"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
)

type Engine interface {
	Start()
	Stop()
	Config() *config.Config
	Server() Server
	SessionManager() SessionManager
	SlotManager() SlotManager
	Cache() Cache
	Ctx() context.Context
}

type Server interface {
	ID() uint32
	Start()
	Stop()
	Send(pkt *packet.RTDEXPacket, dstAddr *net.UDPAddr) error
}

type Cache interface {
	Set(name string, value *CacheItem)
	Get(name string) *CacheItem
	GetAll() []*CacheItem
	Housekeeping()
}

type CacheItem struct {
	Name     string
	Data     []byte
	Size     int
	Expiry   time.Time
	Checksum uint32
}

type SessionManager interface {
	Start()
	CreateSession(id uint32, addr *net.UDPAddr) Session
	GetSession(id uint32) Session
	GetAllSessions() []Session
	Housekeeping()
}

type Session interface {
	Start()
	Stop()
	ID() uint32
	Lifetime() int
	RemoteAddr() string
	UpdateRemoteAddr(addr *net.UDPAddr)
	HandlePacket(pkt *packet.RTDEXPacket)
}

type SlotManager interface {
	Start()
	Stop()
	Slot() int
	SlotSignal() <-chan int
}
