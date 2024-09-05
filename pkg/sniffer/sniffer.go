package sniffer

import (
	"log"
	"sync/atomic"

	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
)

type PacketSniffer struct {
	engine     core.Engine
	count      int
	packetChan chan *core.PacketMeta
	capacity   int
	stopped    atomic.Bool
	logger     *log.Logger
}

func NewPacketSniffer(engine core.Engine) *PacketSniffer {
	s := &PacketSniffer{
		engine:     engine,
		packetChan: make(chan *core.PacketMeta, engine.Config().PacketSnifferCapacity),
		capacity:   engine.Config().PacketSnifferCapacity,
		logger:     log.New(log.Writer(), "[PacketSniffer] ", 0),
	}
	go func() {
		<-engine.Ctx().Done()
		s.stopped.Store(true)
		close(s.packetChan)
	}()
	return s
}

func (s *PacketSniffer) Add(pkt *packet.RTDEXPacket) {
	if s.stopped.Load() {
		return
	}
	s.count++
	metadata := &core.PacketMeta{
		Count:         s.count,
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

	select {
	case s.packetChan <- metadata:
		// Packet added to channel
	default:
		// Channel is full, discard oldest packet and add new one
		<-s.packetChan
		s.packetChan <- metadata
	}
}

func (s *PacketSniffer) Stream() <-chan *core.PacketMeta {
	return s.packetChan
}
