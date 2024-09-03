package sniffer

import (
	"log"
	"sync"

	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
)

type PacketSniffer struct {
	engine   core.Engine
	packets  []*core.PacketMeta
	mu       sync.RWMutex
	capacity int
	logger   *log.Logger
}

func NewPacketSniffer(engine core.Engine) *PacketSniffer {
	s := &PacketSniffer{
		engine:   engine,
		packets:  make([]*core.PacketMeta, 0, engine.Config().PacketSnifferCapacity),
		capacity: engine.Config().PacketSnifferCapacity,
		logger:   log.New(log.Writer(), "[PacketSniffer] ", 0),
	}
	go func() {
		<-engine.Ctx().Done()
		s.Clear()
	}()
	return s
}

func (s *PacketSniffer) Add(pkt *packet.RTDEXPacket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata := &core.PacketMeta{
		UID:           pkt.GetHeader().PacketUid,
		Type:          pkt.GetHeader().PacketType,
		Src:           pkt.GetHeader().SourceId,
		Dst:           pkt.GetHeader().DestinationId,
		Seq:           pkt.GetHeader().SequenceNumber,
		Priority:      pkt.GetHeader().Priority,
		Timestamp:     pkt.GetHeader().Timestamp,
		PayloadLength: pkt.GetHeader().PayloadLength,
	}
	if pkt.GetHeader().PacketType == packet.PacketType_DATA_CONTENT {
		original := pkt.GetPayload().(*packet.RTDEXPacket_DataContent).DataContent
		metadata.Payload = &packet.RTDEXPacket_DataContent{
			DataContent: &packet.DataContent{
				Name:     original.Name,
				Checksum: original.Checksum,
				Data:     []byte("..."),
			},
		}
	} else {
		metadata.Payload = pkt.GetPayload()
	}
	if len(s.packets) >= s.capacity {
		s.packets = append(s.packets[1:], metadata)
	} else {
		s.packets = append(s.packets, metadata)
	}
}

func (s *PacketSniffer) Get(startIndex, endIndex int) []*core.PacketMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if startIndex < 0 {
		return nil
	}
	if endIndex > len(s.packets) {
		endIndex = len(s.packets)
	}
	return s.packets[startIndex:endIndex]
}

func (s *PacketSniffer) Clear() {
	s.logger.Println("Clear logged packets")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets = nil
}
