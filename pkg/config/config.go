package config

import "time"

var DefaultConfig = Config{
	UDPAddr:              ":9999",
	PktBufSize:           10240,
	ChunkSize:            8000,
	PktQueueSize:         1024,
	SlotframeSize:        127,
	SlotDuration:         1 * time.Microsecond,
	AckMaxRetries:        3,
	AckTimeout:           50 * time.Millisecond,
	MaxMissedAcks:        3,
	InitialLifetime:      128, // seconds
	HouseKeepingInterval: 500 * time.Millisecond,
}

type Config struct {
	UDPAddr              string
	PktBufSize           int
	ChunkSize            int
	PktQueueSize         int
	SlotframeSize        int
	SlotDuration         time.Duration
	AckMaxRetries        int
	AckTimeout           time.Duration
	MaxMissedAcks        int
	InitialLifetime      int
	HouseKeepingInterval time.Duration
}
