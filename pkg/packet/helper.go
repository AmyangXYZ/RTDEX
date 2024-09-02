package packet

import (
	"math/rand/v2"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	Version = 1
)

var (
	sequenceNumber = 0
)

func getUID() uint32 {
	return rand.Uint32()
}

func getTimestamp() uint64 {
	return uint64(time.Now().UnixNano())
}

func getSeqNo() uint32 {
	sequenceNumber++
	return uint32(sequenceNumber)
}

func getPayloadLength(pkt *RTDEXPacket) uint32 {
	if pkt == nil {
		return 0
	}
	return uint32(proto.Size(pkt) - proto.Size(pkt.Header))
}

func createHeader(packetType PacketType, uid, seqNo, src, dst uint32, priority Priority) *PacketHeader {
	return &PacketHeader{
		ProtocolVersion: Version,
		PacketType:      packetType,
		PacketUid:       uid,
		SequenceNumber:  seqNo,
		SourceId:        src,
		DestinationId:   dst,
		Priority:        priority,
		Timestamp:       getTimestamp(),
	}
}

func CreateJoinRequestPacket(src, dst uint32, id uint32, type_ ClientType, token uint32) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_JOIN_REQUEST, getUID(), getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_JoinRequest{
			JoinRequest: &JoinRequest{
				Id:                  id,
				Type:                type_,
				AuthenticationToken: token,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateJoinResponsePacket(src, dst uint32, sessionToken uint32) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_JOIN_RESPONSE, getUID(), getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_JoinResponse{
			JoinResponse: &JoinResponse{
				SessionToken: sessionToken,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateDataRegisterPacket(src, dst uint32, name string, freshness, size uint64) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_DATA_REGISTER, getUID(), getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_DataRegister{
			DataRegister: &DataRegister{
				Name:      name,
				Freshness: freshness,
				Size:      size,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateDataInterestPacket(src, dst uint32, name string) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_DATA_INTEREST, getUID(), getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_DataInterest{
			DataInterest: &DataInterest{
				Name: name,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateDataContentPacket(src, dst uint32, name string, checksum uint32, data []byte) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_DATA_CONTENT, getUID(), getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_DataContent{
			DataContent: &DataContent{
				Name:     name,
				Checksum: checksum,
				Data:     data,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateAcknowledgementPacket(src, dst, uid uint32, tx_timestamp uint64) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_ACKNOWLEDGEMENT, uid, getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_Acknowledgement{
			Acknowledgement: &Acknowledgement{
				Latency: time.Since(time.Unix(0, int64(tx_timestamp))).Microseconds(),
			},
		},
	}
	// fmt.Println("Latency:", pkt.GetAcknowledgement().Latency)
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateErrorMessagePacket(src, dst, uid uint32, errorCode ErrorCode) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_ERROR_MESSAGE, uid, getSeqNo(), src, dst, Priority_HIGH),
		Payload: &RTDEXPacket_ErrorMessage{
			ErrorMessage: &ErrorMessage{
				ErrorCode: errorCode,
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}

func CreateHeartbeatPacket(src, dst uint32) *RTDEXPacket {
	pkt := &RTDEXPacket{
		Header: createHeader(PacketType_HEARTBEAT, getUID(), getSeqNo(), src, dst, Priority_LOW),
		Payload: &RTDEXPacket_Heartbeat{
			Heartbeat: &Heartbeat{
				Timestamp: getTimestamp(),
			},
		},
	}
	pkt.Header.PayloadLength = getPayloadLength(pkt)
	return pkt
}
