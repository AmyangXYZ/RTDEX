package main

import (
	"log"
	"net"

	"pntaas/packet"

	"google.golang.org/protobuf/proto"
)

type UDPServer struct {
	ID     uint32 `json:"id"`
	lAddr  *net.UDPAddr
	socket *net.UDPConn
	logger *log.Logger
}

func NewPNTServer(pntServiceAddr string, webServiceAddr string) *UDPServer {
	logger := log.New(log.Writer(), "[Server] ", 0)
	lAddr, err := net.ResolveUDPAddr("udp", pntServiceAddr)
	if err != nil {
		logger.Println(err)
		return nil
	}
	return &UDPServer{
		ID:     1,
		logger: logger,
		lAddr:  lAddr,
	}
}

func (s *UDPServer) Start() {
	var err error

	s.socket, err = net.ListenUDP("udp", s.lAddr)
	if err != nil {
		s.logger.Println(err)
		return
	}
	s.logger.Println("PNT Service on", s.lAddr)
	go s.RecvFromRemote()

}

func (s *UDPServer) RecvFromRemote() {
	var buf [PktBufSize]byte
	for {
		n, addr, err := s.socket.ReadFromUDP(buf[0:])
		if err != nil {
			s.logger.Println(err)
			continue
		}
		pkt := &packet.PNTaaSPacket{}
		if err = proto.Unmarshal(buf[:n], pkt); err != nil {
			s.logger.Println("Unmarshaling error: ", err)
			continue
		}

		pktSniffer.Add(pkt)

		if session := sessionMgr.GetSession(pkt.Header.SourceId); session != nil && session.Lifetime > 0 {
			if session.RemoteAddr.String() != addr.String() {
				session.UpdateRemoteAddr(addr)
			}
			session.HandlePacket(pkt)
		} else if pkt.Header.PacketType == packet.PacketType_JOIN_REQUEST {
			token := pkt.GetJoinRequest().AuthenticationToken
			if token == 123 {
				s.logger.Println("New session", pkt.Header.SourceId)
				src := pkt.Header.SourceId
				type_ := pkt.GetJoinRequest().Type
				session := sessionMgr.CreateSession(src, type_, addr)
				go session.Start()
				session.HandlePacket(pkt)
			} else {
				s.logger.Println("Invalid token")
				s.SendToRemote(packet.CreateAcknowledgementPacket(s.ID, pkt.Header.SourceId, pkt.Header.PacketUid, pkt.Header.Timestamp), addr)
				s.SendToRemote(packet.CreateErrorMessagePacket(s.ID, pkt.Header.SourceId, pkt.Header.PacketUid, packet.ErrorCode_AUTHENTICATION_FAILED), addr)
			}
		}
	}
}

func (s *UDPServer) SendToRemote(pkt *packet.PNTaaSPacket, remoteAddr *net.UDPAddr) error {
	pktSniffer.Add(pkt)

	buf, err := proto.Marshal(pkt)
	if err != nil {
		return err
	}
	_, err = s.socket.WriteToUDP(buf, remoteAddr)
	return err
}
