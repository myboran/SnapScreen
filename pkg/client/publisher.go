package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"log"
	"snap-screen/pkg/screen"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type peerSession struct {
	pc *webrtc.PeerConnection
	dc *webrtc.DataChannel
}

type publisherSession struct {
	streamID string
	capture  *screen.Capture
	cfg      PublisherConfig

	ctx    context.Context
	cancel context.CancelFunc

	statusFn func(PublisherStatus, string)

	mu    sync.RWMutex
	ws    *websocket.Conn
	peers map[string]*peerSession
}

var (
	publisherMu     sync.Mutex
	activePublisher *publisherSession
)

// StartPublisher 初始化 WebRTC 推流（Publisher 端）
func StartPublisher(streamID string, capture *screen.Capture, cfg PublisherConfig, statusFn func(PublisherStatus, string)) error {
	if streamID == "" {
		return errors.New("stream id 不能为空")
	}
	if capture == nil {
		return errors.New("capture 不能为空")
	}
	normalizeConfig(&cfg)

	publisherMu.Lock()
	defer publisherMu.Unlock()
	if activePublisher != nil {
		return errors.New("已有推流任务在运行")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &publisherSession{
		streamID: streamID,
		capture:  capture,
		cfg:      cfg,
		ctx:      ctx,
		cancel:   cancel,
		statusFn: statusFn,
		peers:    make(map[string]*peerSession),
	}

	if err := s.connectAndRegister(); err != nil {
		cancel()
		return err
	}

	activePublisher = s
	s.updateStatus(PublisherStatusConnected, "信令连接成功")
	go s.signalReadLoop()
	go s.captureLoop()
	return nil
}

// StopPublisher 停止推流
func StopPublisher() {
	publisherMu.Lock()
	s := activePublisher
	activePublisher = nil
	publisherMu.Unlock()

	if s == nil {
		return
	}
	s.stop()
}

func (s *publisherSession) connectAndRegister() error {
	ws, _, err := websocket.DefaultDialer.Dial(s.cfg.SignalURL, nil)
	if err != nil {
		s.updateStatus(PublisherStatusError, "连接信令失败: "+err.Error())
		return err
	}
	s.mu.Lock()
	s.ws = ws
	s.mu.Unlock()

	reg := signalMessage{
		Type:     "register",
		StreamID: s.streamID,
	}
	if err := s.writeSignal(reg); err != nil {
		s.updateStatus(PublisherStatusError, "注册流失败: "+err.Error())
		return err
	}
	return nil
}

func (s *publisherSession) signalReadLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		_, b, err := s.ws.ReadMessage()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.updateStatus(PublisherStatusError, "信令读取失败: "+err.Error())
			s.stop()
			return
		}

		var msg signalMessage
		if err := json.Unmarshal(b, &msg); err != nil {
			s.updateStatus(PublisherStatusError, "信令解析失败: "+err.Error())
			continue
		}

		switch msg.Type {
		case "success":
			s.updateStatus(PublisherStatusRunning, "已注册 stream，等待 Viewer 订阅")
		case "error":
			s.updateStatus(PublisherStatusError, msg.Error)
		case "offer":
			if err := s.handleOffer(msg); err != nil {
				s.updateStatus(PublisherStatusError, "处理 Offer 失败: "+err.Error())
			}
		case "ice_candidate":
			if err := s.handleRemoteICE(msg); err != nil {
				s.updateStatus(PublisherStatusError, "处理 ICE 失败: "+err.Error())
			}
		}
	}
}

func (s *publisherSession) captureLoop() {
	interval := time.Second / time.Duration(s.cfg.FrameRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// 没有任何订阅者时不做采集和编码，节省 CPU
			s.mu.RLock()
			hasPeers := len(s.peers) > 0
			s.mu.RUnlock()
			if !hasPeers {
				continue
			}
			frame := s.capturedFrame()
			if frame == nil {
				continue
			}
			payload, err := encodeJPEG(frame)
			if err != nil {
				s.updateStatus(PublisherStatusError, "帧编码失败: "+err.Error())
				continue
			}
			s.broadcastFrame(payload)
		}
	}
}

func (s *publisherSession) capturedFrame() *image.RGBA {
	if s.cfg.Width > 0 && s.cfg.Height > 0 {
		return s.capture.CaptureFrameSized(s.cfg.Width, s.cfg.Height)
	}
	return s.capture.CaptureFrame()
}

func (s *publisherSession) broadcastFrame(payload []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, peer := range s.peers {
		if peer.dc != nil && peer.dc.ReadyState() == webrtc.DataChannelStateOpen {
			if err := peer.dc.Send(payload); err != nil {
				log.Println("send frame failed:", err)
			}
		}
	}
}

func (s *publisherSession) handleOffer(msg signalMessage) error {
	if msg.PeerID == "" {
		return errors.New("offer 缺少 peer_id")
	}

	var remote webrtc.SessionDescription
	if err := json.Unmarshal(msg.Data, &remote); err != nil {
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return err
	}

	// Answer 端不主动创建 DataChannel，而是等待 Viewer 创建的通道协商完成后回调
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		s.updateStatus(PublisherStatusRunning, "Viewer DataChannel 已建立: "+msg.PeerID)
		dc.OnClose(func() {
			s.removePeer(msg.PeerID)
			s.updateStatus(PublisherStatusRunning, "Viewer 已断开: "+msg.PeerID)
		})

		s.mu.Lock()
		ps, ok := s.peers[msg.PeerID]
		if !ok {
			ps = &peerSession{pc: pc}
			s.peers[msg.PeerID] = ps
		}
		ps.dc = dc
		s.mu.Unlock()
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, err := json.Marshal(c.ToJSON())
		if err != nil {
			return
		}
		_ = s.writeSignal(signalMessage{
			Type:     "ice_candidate",
			StreamID: s.streamID,
			PeerID:   msg.PeerID,
			Data:     b,
		})
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			s.removePeer(msg.PeerID)
		}
	})
	if err := pc.SetRemoteDescription(remote); err != nil {
		pc.Close()
		return err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return err
	}
	// 等待 ICE 收集完成，确保 SDP 中包含 ice-ufrag 等字段
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	local := pc.LocalDescription()
	if local == nil {
		pc.Close()
		return errors.New("本地 SDP 为空")
	}

	answerBytes, err := json.Marshal(local)
	if err != nil {
		pc.Close()
		return err
	}
	if err := s.writeSignal(signalMessage{
		Type:     "answer",
		StreamID: s.streamID,
		PeerID:   msg.PeerID,
		Data:     answerBytes,
	}); err != nil {
		pc.Close()
		return err
	}

	s.mu.Lock()
	if _, ok := s.peers[msg.PeerID]; !ok {
		s.peers[msg.PeerID] = &peerSession{pc: pc}
	}
	s.mu.Unlock()
	return nil
}

func (s *publisherSession) handleRemoteICE(msg signalMessage) error {
	s.mu.RLock()
	peer, ok := s.peers[msg.PeerID]
	s.mu.RUnlock()
	if !ok || peer.pc == nil {
		return nil
	}
	var cand webrtc.ICECandidateInit
	if err := json.Unmarshal(msg.Data, &cand); err != nil {
		return err
	}
	return peer.pc.AddICECandidate(cand)
}

func (s *publisherSession) removePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	peer, ok := s.peers[peerID]
	if !ok {
		return
	}
	delete(s.peers, peerID)
	if peer.dc != nil {
		peer.dc.Close()
	}
	if peer.pc != nil {
		peer.pc.Close()
	}
}

func (s *publisherSession) stop() {
	s.cancel()
	_ = s.writeSignal(signalMessage{
		Type:     "unregister",
		StreamID: s.streamID,
	})

	s.mu.Lock()
	for peerID, peer := range s.peers {
		if peer.dc != nil {
			peer.dc.Close()
		}
		if peer.pc != nil {
			peer.pc.Close()
		}
		delete(s.peers, peerID)
	}
	if s.ws != nil {
		s.ws.Close()
		s.ws = nil
	}
	s.mu.Unlock()

	s.updateStatus(PublisherStatusStopped, "推流已停止")
}

func (s *publisherSession) writeSignal(msg signalMessage) error {
	s.mu.RLock()
	ws := s.ws
	s.mu.RUnlock()
	if ws == nil {
		return errors.New("信令连接不存在")
	}
	return ws.WriteJSON(msg)
}

func (s *publisherSession) updateStatus(st PublisherStatus, text string) {
	if s.statusFn != nil {
		s.statusFn(st, text)
	}
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	// 适当降低 JPEG 质量以减小单帧大小和编码开销
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
