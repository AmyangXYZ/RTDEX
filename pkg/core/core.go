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
	PacketSniffer() PacketSniffer
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
	ClearAll()
	Housekeeping()
}

type CacheItem struct {
	Name           string         `json:"name"`
	Size           int            `json:"size"`
	Expiry         time.Time      `json:"expiry"`
	Checksum       uint32         `json:"checksum"`
	NumChunks      int            `json:"num_chunks"`
	Chunks         map[int][]byte `json:"-"`
	ChunkChecksums map[int]uint32 `json:"chunk_checksums"`
}

type SessionManager interface {
	Start()
	CreateSession(id uint32, namespace string, addr *net.UDPAddr) Session
	GetSession(id uint32) Session
	GetAllSessions() []Session
	Housekeeping()
}

type Session interface {
	Start()
	Stop()
	ID() uint32
	Namespace() string
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

type PacketMeta struct {
	Count         int               `json:"count"`
	UID           uint32            `json:"uid"`
	Type          packet.PacketType `json:"type"`
	Src           uint32            `json:"src"`
	Dst           uint32            `json:"dst"`
	Seq           uint32            `json:"seq"`
	Priority      packet.Priority   `json:"priority"`
	Timestamp     uint64            `json:"timestamp"`
	PayloadLength uint32            `json:"payload_length"`
	Payload       interface{}       `json:"payload"`
}

type PacketSniffer interface {
	Add(pkt *packet.RTDEXPacket)
	Stream() <-chan *PacketMeta
}
