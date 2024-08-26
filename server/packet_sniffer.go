package main

import (
	"pntaas/packet"
	"sync"
)

type PacketSniffer struct {
	mu      sync.RWMutex
	packets []*packet.PNTaaSPacket
}

func NewPacketSniffer() *PacketSniffer {
	return &PacketSniffer{}
}

func (s *PacketSniffer) Add(p *packet.PNTaaSPacket) {
	s.mu.Lock()
	s.packets = append(s.packets, p)
	s.mu.Unlock()
}

func (s *PacketSniffer) Get(startIndex int) []*packet.PNTaaSPacket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if startIndex < 0 {
		startIndex = 0
	}

	if startIndex >= len(s.packets) {
		return nil
	}

	result := make([]*packet.PNTaaSPacket, len(s.packets)-startIndex)
	copy(result, s.packets[startIndex:])
	return result
}
