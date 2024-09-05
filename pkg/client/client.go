package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	id           uint32
	namespace    string
	cfg          config.Config
	sessionToken uint32
	socket       *net.UDPConn
	connected    bool
	logger       *log.Logger
}

func NewClient(id uint32, namespace string, cfg config.Config) *Client {
	return &Client{
		id:        id,
		namespace: namespace,
		cfg:       cfg,
		logger:    log.New(log.Writer(), fmt.Sprintf("[Client-%d] ", id), 0),
	}
}

func (c *Client) Connect() error {
	addr, err := net.ResolveUDPAddr("udp", c.cfg.ServerAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	c.socket = conn

	sessionToken, err := c.join(c.cfg.AuthToken)
	if err != nil {
		return err
	}

	c.sessionToken = sessionToken
	c.connected = true

	return nil
}

func (c *Client) Disconnect() {
	if c.connected {
		c.connected = false
		c.socket.Close()
		c.logger.Printf("Disconnected from RTDEX engine\n")
	}
}

func (c *Client) Put(name string, data []byte, freshness uint64) error {
	if !c.connected {
		return errors.New("not connected to RTDEX server")
	}

	dataSize := len(data)
	numChunks := (dataSize + c.cfg.ChunkSize - 1) / c.cfg.ChunkSize
	c.logger.Printf("Registering and uploading %s - freshness: %d seconds, size: %d, chunks: %d\n",
		name, freshness, dataSize, numChunks)

	chunks := make([][]byte, numChunks)
	chunkChecksums := make([]uint32, numChunks)
	chunkChecksumsBytes := make([]byte, 4*numChunks)
	for i := 0; i < numChunks; i++ {
		start := i * c.cfg.ChunkSize
		end := start + c.cfg.ChunkSize
		if end > dataSize {
			end = dataSize
		}
		chunk := data[start:end]
		chunks[i] = chunk
		chunkChecksum := crc32.ChecksumIEEE(chunk)
		chunkChecksums[i] = chunkChecksum
		binary.LittleEndian.PutUint32(chunkChecksumsBytes[i*4:], chunkChecksum)
	}
	rootChecksum := crc32.ChecksumIEEE(chunkChecksumsBytes)

	registerPacket := packet.CreateDataRegisterPacket(c.id, c.cfg.ServerID, name, freshness, uint64(dataSize), rootChecksum, uint32(numChunks))
	if !c.sendPacket(registerPacket) {
		return errors.New("failed to register data after maximum retries")
	}

	for i := 0; i < numChunks; i++ {
		chunk := chunks[i]
		chunkChecksum := chunkChecksums[i]
		uploadPacket := packet.CreateDataContentPacket(c.id, c.cfg.ServerID, name, uint32(i), chunkChecksum, chunk)
		if !c.sendPacket(uploadPacket) {
			return errors.New("failed to upload chunk after maximum retries")
		}
		time.Sleep(time.Millisecond)
	}

	return nil
}

func (c *Client) Get(name string) ([]byte, error) {
	if !c.connected {
		return nil, errors.New("not connected to RTDEX server")
	}

	interestPacket := packet.CreateDataInterestPacket(c.id, c.cfg.ServerID, name)

	if !c.sendPacket(interestPacket) {
		c.logger.Printf("Failed to send DATA_INTEREST after maximum retries\n")
		return nil, errors.New("failed to send DATA_INTEREST")
	}

	// Wait for DATA_INTEREST_RESPONSE
	response, err := c.waitForPacket([]packet.PacketType{packet.PacketType_DATA_INTEREST_RESPONSE, packet.PacketType_ERROR_MESSAGE}, c.cfg.ClientResponseTimeout)
	if err != nil {
		return nil, err
	}

	if response.Header.PacketType == packet.PacketType_ERROR_MESSAGE {
		return nil, fmt.Errorf("error retrieving data: %s", response.GetErrorMessage().ErrorCode.String())
	}

	numChunks := response.GetDataInterestResponse().NumChunks
	rootChecksum := response.GetDataInterestResponse().Checksum
	c.logger.Printf("Received DATA_INTEREST_RESPONSE for %s - numChunks: %d, rootChecksum: %X\n", name, numChunks, rootChecksum)
	// Prepare to receive chunks
	chunks := make(map[uint32][]byte)
	chunkChecksums := make(map[uint32]uint32)

	// Receive all chunks
	for i := uint32(0); i < numChunks; i++ {

		chunkContent, err := c.waitForPacket([]packet.PacketType{packet.PacketType_DATA_CONTENT, packet.PacketType_ERROR_MESSAGE}, c.cfg.ClientResponseTimeout)
		if err != nil {
			return nil, err
		}

		if chunkContent.Header.PacketType == packet.PacketType_ERROR_MESSAGE {
			return nil, fmt.Errorf("error retrieving chunk: %s", chunkContent.GetErrorMessage().ErrorCode.String())
		}

		chunkIndex := chunkContent.GetDataContent().ChunkIndex
		chunkData := chunkContent.GetDataContent().Data
		chunkChecksum := chunkContent.GetDataContent().Checksum

		if crc32.ChecksumIEEE(chunkData) != chunkChecksum {
			return nil, fmt.Errorf("chunk %d integrity check failed", chunkIndex)
		}

		chunks[chunkIndex] = chunkData
		chunkChecksums[chunkIndex] = chunkChecksum
	}
	c.logger.Printf("Received all chunks for %s, verifying root checksum\n", name)
	// Verify root checksum
	allChecksums := make([]byte, 4*numChunks)
	for i := uint32(0); i < numChunks; i++ {
		binary.LittleEndian.PutUint32(allChecksums[i*4:], chunkChecksums[i])
	}
	calculatedRootChecksum := crc32.ChecksumIEEE(allChecksums)
	if calculatedRootChecksum != rootChecksum {
		return nil, errors.New("root checksum verification failed")
	}

	// Merge chunks
	var fullData []byte
	for i := uint32(0); i < numChunks; i++ {
		fullData = append(fullData, chunks[i]...)
	}

	return fullData, nil
}

func (c *Client) sendPacket(pkt *packet.RTDEXPacket) bool {
	serializedPacket, err := proto.Marshal(pkt)
	if err != nil {
		return false
	}

	needsAck := pkt.Header.Priority > packet.Priority_LOW &&
		pkt.Header.PacketType != packet.PacketType_ACKNOWLEDGEMENT &&
		pkt.Header.PacketType != packet.PacketType_ERROR_MESSAGE

	for attempt := 0; attempt < c.cfg.AckMaxRetries; attempt++ {
		_, err := c.socket.Write(serializedPacket)
		if err != nil {
			continue
		}

		c.logger.Printf("Send %s-0x%X\n", pkt.Header.PacketType, pkt.Header.PacketUid)

		if !needsAck {
			return true
		}

		ack, err := c.waitForPacket([]packet.PacketType{packet.PacketType_ACKNOWLEDGEMENT}, c.cfg.AckTimeout)
		if err == nil && ack != nil {
			return true
		}

		c.logger.Printf("No ACK received for %s, retrying...\n", pkt.Header.PacketType)
		time.Sleep(c.cfg.RetryDelay)
	}

	return false
}

func (c *Client) receivePacket(timeout time.Duration) (*packet.RTDEXPacket, error) {
	err := c.socket.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, c.cfg.PktBufSize)
	n, _, err := c.socket.ReadFromUDP(buffer)
	if err != nil {
		return nil, err
	}

	pkt := &packet.RTDEXPacket{}
	err = proto.Unmarshal(buffer[:n], pkt)
	if err != nil {
		return nil, err
	}

	c.logger.Printf("Received %s-0x%X\n", pkt.Header.PacketType, pkt.Header.PacketUid)

	if pkt.Header.Priority > packet.Priority_LOW &&
		pkt.Header.PacketType != packet.PacketType_ACKNOWLEDGEMENT &&
		pkt.Header.PacketType != packet.PacketType_ERROR_MESSAGE {
		c.sendAck(pkt)
	}

	return pkt, nil
}

func (c *Client) waitForPacket(expectedTypes []packet.PacketType, timeout time.Duration) (*packet.RTDEXPacket, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timeout waiting for packet")
		default:
			packet, err := c.receivePacket(time.Second)
			if err != nil {
				continue
			}
			for _, expectedType := range expectedTypes {
				if packet.Header.PacketType == expectedType {
					return packet, nil
				}
			}
		}
	}
}

func (c *Client) sendAck(pkt *packet.RTDEXPacket) {
	ackPacket := packet.CreateAcknowledgementPacket(c.id, c.cfg.ServerID, pkt.Header.PacketUid, pkt.Header.Timestamp)
	c.sendPacket(ackPacket)
}

func (c *Client) join(authToken uint32) (uint32, error) {
	joinRequest := packet.CreateJoinRequestPacket(c.id, c.cfg.ServerID, c.id, c.namespace, authToken)

	if !c.sendPacket(joinRequest) {
		return 0, errors.New("failed to join after maximum retries")
	}

	response, err := c.waitForPacket([]packet.PacketType{packet.PacketType_JOIN_RESPONSE, packet.PacketType_ERROR_MESSAGE}, c.cfg.ClientResponseTimeout)
	if err != nil {
		return 0, err
	}

	if response.Header.PacketType == packet.PacketType_ERROR_MESSAGE {
		return 0, fmt.Errorf("join failed with error: %s", response.GetErrorMessage().ErrorCode.String())
	}

	return response.GetJoinResponse().SessionToken, nil
}
