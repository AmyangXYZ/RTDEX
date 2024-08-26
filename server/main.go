package main

import (
	"fmt"
	"time"
)

const (
	pntAddr = ":9999"
	webAddr = ":8080"

	PktBufSize           = 10240
	ChunkSize            = 8000
	PktQueueSize         = 1024
	SlotframeSize        = 127
	SlotDuration         = 1 * time.Microsecond
	AckMaxRetries        = 3
	AckTimeout           = 50 * time.Millisecond
	MaxMissedAcks        = 3
	InitialLifetime      = 128 // seconds
	HouseKeepingInterval = 500 * time.Millisecond
)

var (
	server     = NewPNTServer(pntAddr, webAddr)
	envMgr     = NewEnvironmentManager()
	sessionMgr = NewSessionManager()
	slotMgr    = NewSlotManager()
	pktSniffer = NewPacketSniffer()
	cache      = NewCache()
	webServer  = NewWebServer(webAddr)
)

func main() {
	fmt.Println(envMgr)
	go slotMgr.incrementASN()
	go server.Start()
	if err := webServer.Start(); err != nil {
		fmt.Println(err)
		return
	}
}
