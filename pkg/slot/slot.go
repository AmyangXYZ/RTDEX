package slot

import (
	"log"
	"runtime"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/core"
)

type SlotManager struct {
	engine              core.Engine
	asn                 int
	slotOffset          int
	slotframeSize       int
	slotDuration        time.Duration
	slotIncrementSignal chan int
	logger              *log.Logger
}

func NewSlotManager(engine core.Engine) core.SlotManager {
	return &SlotManager{
		engine:              engine,
		slotframeSize:       engine.Config().SlotframeSize,
		slotDuration:        engine.Config().SlotDuration,
		slotIncrementSignal: make(chan int),
		logger:              log.New(log.Writer(), "[SlotMgr] ", 0),
	}
}

func (s *SlotManager) spinWait() {
	start := time.Now()
	for time.Since(start) < s.slotDuration {
		runtime.Gosched()
	}
}

func (s *SlotManager) Start() {
	for {
		select {
		case <-s.engine.Ctx().Done():
			s.Stop()
			return
		default:
			s.slotOffset = s.asn % s.slotframeSize
			s.spinWait()
			s.slotIncrementSignal <- s.slotOffset
			s.asn++
		}
	}
}

func (s *SlotManager) Stop() {
	s.logger.Println("Stopping slot manager")
	close(s.slotIncrementSignal)
}

func (s *SlotManager) Slot() int {
	return s.slotOffset
}

func (s *SlotManager) SlotSignal() <-chan int {
	return s.slotIncrementSignal
}
