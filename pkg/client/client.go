package client

import (
	"context"
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
	cfg          config.Config
	sessionToken uint32
	socket       *net.UDPConn
	connected    bool
	logger       *log.Logger
}

func NewClient(id uint32, deviceType string, cfg config.Config) *Client {
	return &Client{
		id:     id,
		cfg:    cfg,
		logger: log.New(log.Writer(), fmt.Sprintf("[Client-%d] ", id), 0),
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
		c.logger.Printf("Disconnected from RTDEX server\n")
	}
}

func (c *Client) Put(name string, data []byte, freshness uint64) error {
	if !c.connected {
		return errors.New("not connected to RTDEX server")
	}

	return c.registerAndUploadData(name, data, freshness)
}

func (c *Client) Get(name string) ([]byte, error) {
	if !c.connected {
		return nil, errors.New("not connected to RTDEX server")
	}

	interestPacket := packet.CreateDataInterestPacket(c.id, 1, name)

	if !c.sendPacket(interestPacket) {
		c.logger.Printf("Failed to send DATA_INTEREST after maximum retries\n")
		return nil, errors.New("failed to send DATA_INTEREST")
	}

	response, err := c.waitForPacket([]packet.PacketType{packet.PacketType_DATA_CONTENT, packet.PacketType_ERROR_MESSAGE}, c.cfg.ClientResponseTimeout)
	if err != nil {
		return nil, err
	}

	if response.Header.PacketType == packet.PacketType_ERROR_MESSAGE {
		return nil, fmt.Errorf("error retrieving data: %s", response.GetErrorMessage().ErrorCode.String())
	}

	if crc32.ChecksumIEEE(response.GetDataContent().Data) != response.GetDataContent().Checksum {
		return nil, errors.New("data integrity check failed")
	}

	return response.GetDataContent().Data, nil
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
	joinRequest := packet.CreateJoinRequestPacket(c.id, c.cfg.ServerID, c.id, 1, authToken)

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

func (c *Client) registerAndUploadData(name string, data []byte, freshness uint64) error {
	dataSize := len(data)
	numChunks := (dataSize + c.cfg.ChunkSize - 1) / c.cfg.ChunkSize
	c.logger.Printf("Registering and uploading %s - freshness: %d seconds, size: %d, chunks: %d\n",
		name, freshness, dataSize, numChunks)

	for chunkID := 0; chunkID < numChunks; chunkID++ {
		start := chunkID * c.cfg.ChunkSize
		end := start + c.cfg.ChunkSize
		if end > dataSize {
			end = dataSize
		}
		chunk := data[start:end]
		chunkName := name
		if numChunks > 1 {
			chunkName = fmt.Sprintf("%s/%d", name, chunkID+1)
		}
		chunkSize := len(chunk)
		checksum := crc32.ChecksumIEEE(chunk)

		err := c.registerData(chunkName, chunk, uint64(chunkSize), freshness, checksum)
		if err != nil {
			return err
		}
		time.Sleep(time.Millisecond)
	}

	return nil
}

func (c *Client) registerData(dataName string, data []byte, dataSize, freshness uint64, checksum uint32) error {
	c.logger.Printf("Registering data: %s, size: %d, freshness: %d seconds, checksum: %X\n",
		dataName, dataSize, freshness, checksum)

	registerPacket := packet.CreateDataRegisterPacket(c.id, c.cfg.ServerID, dataName, freshness, dataSize)

	if c.sendPacket(registerPacket) {
		return c.uploadData(dataName, data, checksum)
	}
	return errors.New("failed to register data after maximum retries")
}

func (c *Client) uploadData(dataName string, data []byte, checksum uint32) error {
	c.logger.Printf("Uploading data: %s, checksum: %X\n", dataName, checksum)

	uploadPacket := packet.CreateDataContentPacket(c.id, c.cfg.ServerID, dataName, checksum, data)

	if !c.sendPacket(uploadPacket) {
		return errors.New("failed to upload data after maximum retries")
	}
	return nil
}
