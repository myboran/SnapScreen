package server

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Server 信令服务器
type Server struct {
	Clients  map[*Client]bool
	Streams  map[string]*PublisherStream // streamID -> Publisher
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		Streams: make(map[string]*PublisherStream),
		Clients: make(map[*Client]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// ServeWS WebSocket 入口
func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := NewClient(conn, s)

	s.registerClient(client)

	go client.writePump()
	go client.readPump()
}

func (s *Server) registerClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clients[c] = true
	log.Printf("registerClient: %s", c.PeerID)
}

func (s *Server) unregisterClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.Printf("unregisterClient: %s", c.PeerID)
	// 删除客户端
	delete(s.Clients, c)

	if c.Role == "publisher" && c.StreamID != "" {
		// 删除流，同时通知所有 viewer
		if stream, ok := s.Streams[c.StreamID]; ok {
			for _, viewer := range stream.Viewers {
				viewer.SendError("stream removed")
				viewer.StreamID = ""
			}
		}
		delete(s.Streams, c.StreamID)
	}

	if c.Role == "viewer" && c.StreamID != "" {
		// 从流中移除 viewer
		if stream, ok := s.Streams[c.StreamID]; ok {
			delete(stream.Viewers, c.PeerID)
		}
	}

	// 关闭发送通道
	close(c.Send)
}
