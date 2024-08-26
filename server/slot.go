package main

import (
	"runtime"
	"time"
)

type SlotManager struct {
	ASN                 int           `json:"asn"`
	SlotOffset          int           `json:"slot_offset"`
	SlotframeSize       int           `json:"slotframe_size"`
	SlotDuration        time.Duration `json:"slot_duration"`
	SlotIncrementSignal chan int
}

func NewSlotManager() *SlotManager {
	return &SlotManager{
		SlotframeSize:       SlotframeSize,
		SlotDuration:        SlotDuration,
		SlotIncrementSignal: make(chan int),
	}
}

func (s *SlotManager) spinWait() {
	start := time.Now()
	for time.Since(start) < s.SlotDuration {
		runtime.Gosched()
	}
}

func (s *SlotManager) incrementASN() {
	for {
		s.SlotOffset = s.ASN % s.SlotframeSize
		s.spinWait()
		s.SlotIncrementSignal <- s.SlotOffset
		s.ASN++
	}
}
