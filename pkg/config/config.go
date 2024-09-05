package config

import "time"

var DefaultConfig = Config{
	ServerID:              1,
	ServerAddr:            ":9999",
	AuthToken:             123,
	PktBufSize:            10240,
	ChunkSize:             8192,
	PktQueueSize:          1024,
	SlotframeSize:         127,
	SlotDuration:          1 * time.Microsecond,
	AckMaxRetries:         3,
	AckTimeout:            50 * time.Millisecond,
	MaxMissedAcks:         3,
	SessionLifetime:       128, // seconds
	PacketSnifferCapacity: 500,
	HeartbeatInterval:     10 * time.Millisecond,
	RetryDelay:            500 * time.Millisecond,
	ClientResponseTimeout: 3 * time.Second,
}

type Config struct {
	ServerID              uint32
	ServerAddr            string
	AuthToken             uint32
	PktBufSize            int
	ChunkSize             int
	PktQueueSize          int
	SlotframeSize         int
	SlotDuration          time.Duration
	AckMaxRetries         int
	AckTimeout            time.Duration
	MaxMissedAcks         int
	SessionLifetime       int
	PacketSnifferCapacity int
	HeartbeatInterval     time.Duration
	RetryDelay            time.Duration
	ClientResponseTimeout time.Duration
}
