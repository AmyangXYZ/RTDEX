package engine

import (
	"context"
	"log"

	"github.com/AmyangXYZ/rtdex/pkg/cache"
	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/server"
	"github.com/AmyangXYZ/rtdex/pkg/session"
	"github.com/AmyangXYZ/rtdex/pkg/slot"
	"github.com/AmyangXYZ/rtdex/pkg/sniffer"
)

type RTDEXEngine struct {
	cfg            config.Config
	server         core.Server
	sessionManager core.SessionManager
	slotManager    core.SlotManager
	cache          core.Cache
	ctx            context.Context
	packetSniffer  core.PacketSniffer
	cancel         context.CancelFunc
	logger         *log.Logger
}

func NewEngine(cfg config.Config) *RTDEXEngine {
	engine := &RTDEXEngine{
		cfg:    cfg,
		logger: log.New(log.Writer(), "[Engine] ", 0),
	}
	engine.ctx, engine.cancel = context.WithCancel(context.Background())
	engine.cache = cache.NewCache(engine)
	engine.server = server.NewServer(engine)
	engine.sessionManager = session.NewSessionManager(engine)
	engine.slotManager = slot.NewSlotManager(engine)
	engine.packetSniffer = sniffer.NewPacketSniffer(engine)
	return engine
}

func (e *RTDEXEngine) Config() *config.Config {
	return &e.cfg
}

func (e *RTDEXEngine) Start() {
	go e.sessionManager.Start()
	go e.server.Start()
	go e.slotManager.Start()
	select {}
}

func (e *RTDEXEngine) Stop() {
	e.logger.Println("Stop RTDEX engine")
	e.cancel()
}

func (e *RTDEXEngine) Server() core.Server {
	return e.server
}

func (e *RTDEXEngine) SessionManager() core.SessionManager {
	return e.sessionManager
}

func (e *RTDEXEngine) SlotManager() core.SlotManager {
	return e.slotManager
}

func (e *RTDEXEngine) Cache() core.Cache {
	return e.cache
}

func (e *RTDEXEngine) PacketSniffer() core.PacketSniffer {
	return e.packetSniffer
}

func (e *RTDEXEngine) Ctx() context.Context {
	return e.ctx
}
