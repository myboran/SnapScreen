package client

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

// PublisherStatus 表示 Publisher 当前推流状态，用于 UI 展示
type PublisherStatus string

const (
	PublisherStatusDisconnected PublisherStatus = "未连接"
	PublisherStatusConnected    PublisherStatus = "已连接"
	PublisherStatusError        PublisherStatus = "错误"
	PublisherStatusRunning      PublisherStatus = "推流中"
	PublisherStatusStopped      PublisherStatus = "已停止"
)

// PublisherConfig 控制推流侧的基础参数
type PublisherConfig struct {
	SignalURL string
	FrameRate int
	Width     int
	Height    int
}

// signalMessage 是客户端与信令服务器之间的 JSON 消息结构
type signalMessage struct {
	Type     string          `json:"type"`
	StreamID string          `json:"stream_id,omitempty"`
	PeerID   string          `json:"peer_id,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// 默认信令地址，Publisher / Viewer 共用
var defaultSignalURL = "ws://127.0.0.1:8080/ws"

// normalizeConfig 填充 PublisherConfig 的默认值并做安全裁剪
func normalizeConfig(cfg *PublisherConfig) {
	if cfg.SignalURL == "" {
		cfg.SignalURL = defaultSignalURL
	}
	if cfg.FrameRate <= 0 {
		// 默认帧率调低到 30，兼顾流畅度与 CPU 占用
		cfg.FrameRate = 30
	}
	// 将帧率上限从 60 下调到 30，避免在高分辨率屏幕上过载
	if cfg.FrameRate > 30 {
		cfg.FrameRate = 30
	}
	if cfg.Width < 0 {
		cfg.Width = 0
	}
	if cfg.Height < 0 {
		cfg.Height = 0
	}
}

// FetchStreamList 拉取当前可用的流 ID 列表
func FetchStreamList(signalURL string) ([]string, error) {
	if signalURL == "" {
		signalURL = defaultSignalURL
	}
	ws, _, err := websocket.DefaultDialer.Dial(signalURL, nil)
	if err != nil {
		return nil, err
	}
	defer ws.Close()

	req := signalMessage{Type: "list_streams"}
	if err := ws.WriteJSON(req); err != nil {
		return nil, err
	}

	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg signalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	if msg.Type != "stream_list" {
		return nil, errors.New("unexpected message type: " + msg.Type)
	}

	var ids []string
	if err := json.Unmarshal(msg.Data, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}
