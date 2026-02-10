package server

import (
	"encoding/json"
	"log"
	sig "snap-screen/pkg/signal"
	"snap-screen/pkg/utils"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	Conn     *websocket.Conn
	Send     chan []byte
	Role     string // "publisher" 或 "viewer"
	StreamID string
	PeerID   string
	Server   *Server
}

func NewClient(conn *websocket.Conn, s *Server) *Client {
	return &Client{
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Server: s,
		PeerID: utils.GenID(),
	}
}

func (c *Client) readPump() {
	defer func() {
		c.Server.unregisterClient(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512 * 1024)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msgBytes, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				log.Println("WebSocket read error:", err)
			}
			break
		}
		c.handleMessage(msgBytes)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)
			w.Close()
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			c.Conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
}

func (c *Client) handleMessage(msgBytes []byte) {
	// 1️⃣ 解析消息
	var msg sig.Message
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		c.SendError("invalid message format")
		return
	}

	// 2️⃣ 调用 Server 的 Router 分发
	c.Server.RouteMessage(c, &msg)
}
