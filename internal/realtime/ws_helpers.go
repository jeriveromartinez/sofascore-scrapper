package realtime

import (
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// pbWsHello is a tiny adapter so handler.go can build WsHello
// without importing the gen package in every file. It also keeps
// the construction site greppable.
type pbWsHello struct {
	DeviceId   uint64
	ServerTime int64
}

func (h *pbWsHello) toProto() *pb.WsFrame {
	return &pb.WsFrame{Payload: &pb.WsFrame_Hello{Hello: &pb.WsHello{
		DeviceId:   h.DeviceId,
		ServerTime: h.ServerTime,
	}}}
}

type pbWsPing struct {
	SentAt int64
}

func (p pbWsPing) toProto() *pb.WsFrame {
	return &pb.WsFrame{Payload: &pb.WsFrame_Ping{Ping: &pb.WsPing{
		SentAt: p.SentAt,
	}}}
}
