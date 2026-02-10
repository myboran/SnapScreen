package server

import (
	"encoding/json"
	"log"
	sig "snap-screen/pkg/signal"
)

// RouteMessage 根据消息类型路由处理
func (s *Server) RouteMessage(c *Client, msg *sig.Message) {
	log.Println("RouteMessage", msg.Type)
	switch msg.Type {
	case sig.MsgTypeRegister:
		s.handleRegister(c, msg)
	case sig.MsgTypeUnregister:
		s.handleUnregister(c, msg)
	case sig.MsgTypeListStreams:
		s.handleListStreams(c)
	case sig.MsgTypeSubscribe:
		s.handleSubscribe(c, msg)
	case sig.MsgTypeUnsubscribe:
		s.handleUnsubscribe(c, msg)
	case sig.MsgTypeOffer, sig.MsgTypeAnswer, sig.MsgTypeICECandidate:
		s.forwardSignal(c, msg)
	default:
		c.SendError("unknown message type")
	}
}

// -------------------- 各种处理函数 --------------------

func (s *Server) handleRegister(c *Client, msg *sig.Message) {
	if msg.StreamID == "" {
		c.SendError("stream_id required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Streams[msg.StreamID]; exists {
		c.SendError("stream_id already registered")
		return
	}

	c.Role = "publisher"
	c.StreamID = msg.StreamID
	s.Streams[msg.StreamID] = NewPublisherStream(c)

	c.SendSuccess("stream registered")
}

func (s *Server) handleUnregister(c *Client, msg *sig.Message) {
	if msg.StreamID == "" {
		c.SendError("stream_id required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, exists := s.Streams[msg.StreamID]; exists && stream.Publisher == c {
		for peerID, viewer := range stream.Viewers {
			viewer.SendError("stream removed")
			delete(stream.Viewers, peerID)
		}
		delete(s.Streams, msg.StreamID)
		c.StreamID = ""
		c.SendSuccess("stream unregistered")
	} else {
		c.SendError("stream not found or not publisher")
	}
}

func (s *Server) handleListStreams(c *Client) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	streamIDs := []string{}
	for streamID := range s.Streams {
		streamIDs = append(streamIDs, streamID)
	}

	msg := &sig.Message{
		Type: sig.MsgTypeStreamList,
		Data: streamIDs,
	}
	c.SendJSON(msg)
}

func (s *Server) handleSubscribe(c *Client, msg *sig.Message) {
	if msg.StreamID == "" {
		c.SendError("stream_id required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.Streams[msg.StreamID]
	if !exists {
		c.SendError("stream not found")
		return
	}

	c.Role = "viewer"
	c.StreamID = msg.StreamID
	if stream.Viewers == nil {
		stream.Viewers = make(map[string]*Client)
	}
	c.PeerID = msg.PeerID
	stream.Viewers[c.PeerID] = c

	c.SendSuccess("subscribed")
}

func (s *Server) handleUnsubscribe(c *Client, msg *sig.Message) {
	if msg.StreamID == "" {
		c.SendError("stream_id required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.Streams[msg.StreamID]
	if !exists {
		c.SendError("stream not found")
		return
	}

	if stream.Viewers != nil {
		delete(stream.Viewers, c.PeerID)
	}
	c.StreamID = ""
	c.SendSuccess("unsubscribed")
}

// forwardSignal 转发 WebRTC 信令消息
func (s *Server) forwardSignal(c *Client, msg *sig.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch msg.Type {
	case sig.MsgTypeOffer:
		// Viewer → Publisher
		stream, exists := s.Streams[msg.StreamID]
		if !exists {
			c.SendError("stream not found")
			return
		}
		if stream.Publisher != nil {
			stream.Publisher.SendJSON(msg)
		}

	case sig.MsgTypeAnswer:
		// Publisher → Viewer
		stream, exists := s.Streams[msg.StreamID]
		if !exists {
			c.SendError("stream not found")
			return
		}
		if viewer, ok := stream.Viewers[msg.PeerID]; ok {
			viewer.SendJSON(msg)
		}

	case sig.MsgTypeICECandidate:
		// 双向都可以
		stream, exists := s.Streams[msg.StreamID]
		if !exists {
			c.SendError("stream not found")
			return
		}
		switch c.Role {
		case "viewer":
			if stream.Publisher != nil {
				stream.Publisher.SendJSON(msg)
			}
		case "publisher":
			if viewer, ok := stream.Viewers[msg.PeerID]; ok {
				viewer.SendJSON(msg)
			}
		}
	}
}

// -------------------- Client 辅助方法 --------------------

func (c *Client) SendError(errMsg string) {
	msg := &sig.Message{
		Type:  sig.MsgTypeError,
		Error: errMsg,
	}
	c.SendJSON(msg)
}

func (c *Client) SendSuccess(info string) {
	msg := &sig.Message{
		Type: sig.MsgTypeSuccess,
		Data: map[string]string{"message": info},
	}
	c.SendJSON(msg)
}

func (c *Client) SendJSON(msg *sig.Message) {
	b, err := json.Marshal(msg)
	if err != nil {
		log.Println("SendJSON marshal error:", err)
		return
	}
	select {
	case c.Send <- b:
	default:
	}
}
