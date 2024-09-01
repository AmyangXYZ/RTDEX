package server

import (
	"log"
	"net"

	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/packet"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	engine core.Engine
	id     uint32
	addr   *net.UDPAddr
	socket *net.UDPConn
	logger *log.Logger
}

func NewServer(engine core.Engine) core.Server {
	logger := log.New(log.Writer(), "[Server] ", 0)
	addr, err := net.ResolveUDPAddr("udp", engine.Config().UDPAddr)
	if err != nil {
		logger.Println(err)
		return nil
	}
	return &Server{
		engine: engine,
		id:     1,
		logger: logger,
		addr:   addr,
	}
}

func (s *Server) ID() uint32 {
	return s.id
}

func (s *Server) Start() {
	var err error

	s.socket, err = net.ListenUDP("udp", s.addr)
	if err != nil {
		s.logger.Println(err)
		return
	}
	s.logger.Println("Server listening on", s.addr)

	buf := make([]byte, s.engine.Config().PktBufSize)
	for {
		select {
		case <-s.engine.Ctx().Done():
			s.Stop()
			return
		default:
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

			if session := s.engine.SessionManager().GetSession(pkt.Header.SourceId); session != nil && session.Lifetime() > 0 {
				if session.RemoteAddr() != addr.String() {
					session.UpdateRemoteAddr(addr)
				}
				session.HandlePacket(pkt)
			} else if pkt.Header.PacketType == packet.PacketType_JOIN_REQUEST {
				token := pkt.GetJoinRequest().AuthenticationToken
				if token == 123 {
					s.logger.Println("New session", pkt.Header.SourceId)
					src := pkt.Header.SourceId
					// type_ := pkt.GetJoinRequest().Type
					session := s.engine.SessionManager().CreateSession(src, addr)
					go session.Start()
					session.HandlePacket(pkt)
				} else {
					s.logger.Println("Invalid token")
					s.Send(packet.CreateAcknowledgementPacket(s.id, pkt.Header.SourceId, pkt.Header.PacketUid, pkt.Header.Timestamp), addr)
					s.Send(packet.CreateErrorMessagePacket(s.id, pkt.Header.SourceId, pkt.Header.PacketUid, packet.ErrorCode_AUTHENTICATION_FAILED), addr)
				}
			}
		}
	}

}

func (s *Server) Stop() {
	s.logger.Println("Server stopping")
	s.socket.Close()
}

func (s *Server) Send(pkt *packet.PNTaaSPacket, dstAddr *net.UDPAddr) error {
	buf, err := proto.Marshal(pkt)
	if err != nil {
		return err
	}
	_, err = s.socket.WriteToUDP(buf, dstAddr)
	return err
}
