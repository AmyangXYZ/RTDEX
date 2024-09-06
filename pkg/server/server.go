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
	addr, err := net.ResolveUDPAddr("udp", engine.Config().ServerAddr)
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
			pkt := packet.PacketPool.Get().(*packet.RTDEXPacket)
			pkt.Reset()
			if err = proto.Unmarshal(buf[:n], pkt); err != nil {
				s.logger.Println("Unmarshaling error: ", err)
				packet.PacketPool.Put(pkt)
				continue
			}
			s.engine.PacketSniffer().Add(pkt)
			if session := s.engine.SessionManager().GetSession(pkt.GetHeader().SourceId); session != nil {
				if session.RemoteAddr() != addr.String() {
					session.UpdateRemoteAddr(addr)
				}
				session.HandlePacket(pkt)
			} else if pkt.GetHeader().PacketType == packet.PacketType_JOIN_REQUEST {
				token := pkt.GetJoinRequest().AuthenticationToken
				if token == s.engine.Config().AuthToken {
					id := pkt.GetHeader().SourceId
					namespace := pkt.GetJoinRequest().Namespace
					s.logger.Printf("New session id:%d, namespace:%s", id, namespace)
					session := s.engine.SessionManager().CreateSession(id, namespace, addr)
					go session.Start()
					session.HandlePacket(pkt)
				} else {
					s.logger.Println("Invalid token")
					s.Send(packet.CreateAcknowledgementPacket(s.id, pkt.GetHeader().SourceId, pkt.GetHeader().PacketUid, pkt.GetHeader().Timestamp), addr)
					s.Send(packet.CreateErrorMessagePacket(s.id, pkt.GetHeader().SourceId, pkt.GetHeader().PacketUid, packet.ErrorCode_AUTHENTICATION_FAILED), addr)
				}
			}
			packet.PacketPool.Put(pkt)
		}
	}

}

func (s *Server) Stop() {
	s.logger.Println("Stop server")
	s.socket.Close()
}

func (s *Server) Send(pkt *packet.RTDEXPacket, dstAddr *net.UDPAddr) error {
	s.engine.PacketSniffer().Add(pkt)
	buf, err := proto.Marshal(pkt)
	if err != nil {
		return err
	}
	_, err = s.socket.WriteToUDP(buf, dstAddr)
	return err
}
