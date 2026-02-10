package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"log"
	"snap-screen/pkg/utils"
	"sync"

	"fyne.io/fyne/v2/canvas"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type viewerSession struct {
	streamID  string
	peerID    string
	signalURL string

	ctx    context.Context
	cancel context.CancelFunc

	ws *websocket.Conn
	pc *webrtc.PeerConnection
	dc *webrtc.DataChannel

	img *canvas.Image

	mu sync.Mutex

	remoteSet   bool
	pendingICEs []webrtc.ICECandidateInit
}

var (
	viewerMu     sync.Mutex
	activeViewer *viewerSession
)

// StartViewer 初始化 WebRTC 观看
func StartViewer(streamID string, img *canvas.Image, signalURL string) error {
	if streamID == "" {
		return errors.New("stream id 不能为空")
	}
	if signalURL == "" {
		signalURL = defaultSignalURL
	}
	if img == nil {
		return errors.New("image 组件不能为空")
	}

	viewerMu.Lock()
	defer viewerMu.Unlock()
	if activeViewer != nil {
		// 先关闭旧的
		activeViewer.stop()
		activeViewer = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &viewerSession{
		streamID:  streamID,
		peerID:    utils.GenID(),
		signalURL: signalURL,
		ctx:       ctx,
		cancel:    cancel,
		img:       img,
	}

	if err := s.connectAndSubscribe(); err != nil {
		cancel()
		return err
	}
	if err := s.createPeerConnection(); err != nil {
		s.stop()
		return err
	}
	if err := s.createAndSendOffer(); err != nil {
		s.stop()
		return err
	}

	activeViewer = s
	go s.readLoop()
	return nil
}

// StopViewer 停止观看
func StopViewer() {
	viewerMu.Lock()
	s := activeViewer
	activeViewer = nil
	viewerMu.Unlock()

	if s != nil {
		s.stop()
	}
}

// -------------------- Viewer 内部实现 --------------------

func (s *viewerSession) connectAndSubscribe() error {
	ws, _, err := websocket.DefaultDialer.Dial(s.signalURL, nil)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ws = ws
	s.mu.Unlock()

	msg := signalMessage{
		Type:     "subscribe",
		StreamID: s.streamID,
		PeerID:   s.peerID,
	}
	return s.writeSignal(msg)
}

func (s *viewerSession) createPeerConnection() error {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return err
	}

	pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil {
			return
		}
		b, err := json.Marshal(cand.ToJSON())
		if err != nil {
			return
		}
		_ = s.writeSignal(signalMessage{
			Type:     "ice_candidate",
			StreamID: s.streamID,
			PeerID:   s.peerID,
			Data:     b,
		})
	})

	// 作为 Offer 端，创建 DataChannel，这样 SCTP m= 行会出现在 Offer SDP 中
	dc, err := pc.CreateDataChannel("screen-frames", nil)
	if err != nil {
		log.Println("viewer CreateDataChannel error:", err)
	} else {
		s.mu.Lock()
		s.dc = dc
		s.mu.Unlock()

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			img, err := jpeg.Decode(bytes.NewReader(msg.Data))
			if err != nil {
				log.Println("decode frame error:", err)
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.img != nil {
				s.img.Image = img
				s.img.Refresh()
			}
		})
	}

	s.mu.Lock()
	s.pc = pc
	s.mu.Unlock()
	return nil
}

func (s *viewerSession) createAndSendOffer() error {
	s.mu.Lock()
	pc := s.pc
	s.mu.Unlock()
	if pc == nil {
		return errors.New("peer connection 尚未创建")
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return err
	}
	// 等待 ICE 收集完成，确保 SDP 中包含 ice-ufrag 等字段
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	local := pc.LocalDescription()
	if local == nil {
		return errors.New("本地 SDP 为空")
	}

	b, err := json.Marshal(local)
	if err != nil {
		return err
	}
	return s.writeSignal(signalMessage{
		Type:     "offer",
		StreamID: s.streamID,
		PeerID:   s.peerID,
		Data:     b,
	})
}

func (s *viewerSession) readLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		_, data, err := s.ws.ReadMessage()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			log.Println("viewer read signal error:", err)
			s.stop()
			return
		}

		var msg signalMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Println("viewer parse signal error:", err)
			continue
		}
		switch msg.Type {
		case "answer":
			if err := s.handleAnswer(msg); err != nil {
				log.Println("handle answer error:", err)
			}
		case "ice_candidate":
			if err := s.handleICE(msg); err != nil {
				log.Println("handle ice error:", err)
			}
		case "error":
			log.Println("viewer received error:", msg.Error)
		}
	}
}

func (s *viewerSession) handleAnswer(msg signalMessage) error {
	s.mu.Lock()
	pc := s.pc
	if pc == nil {
		s.mu.Unlock()
		return errors.New("peer connection 尚未创建")
	}
	s.mu.Unlock()

	var ans webrtc.SessionDescription
	if err := json.Unmarshal(msg.Data, &ans); err != nil {
		return err
	}
	if err := pc.SetRemoteDescription(ans); err != nil {
		log.Println("viewer SetRemoteDescription error:", err)
		return err
	}

	// 远端 SDP 设置完成后，把之前缓存的 ICE 一次性加进去
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteSet = true
	for _, cand := range s.pendingICEs {
		if err := pc.AddICECandidate(cand); err != nil {
			log.Println("flush buffered ICE error:", err)
		}
	}
	s.pendingICEs = nil
	return nil
}

func (s *viewerSession) handleICE(msg signalMessage) error {
	s.mu.Lock()
	pc := s.pc
	remoteSet := s.remoteSet
	s.mu.Unlock()
	if pc == nil {
		return nil
	}
	var cand webrtc.ICECandidateInit
	if err := json.Unmarshal(msg.Data, &cand); err != nil {
		return err
	}
	if !remoteSet {
		// 远端 SDP 还没设置好，先缓存，等 handleAnswer 之后再批量加入
		s.mu.Lock()
		s.pendingICEs = append(s.pendingICEs, cand)
		s.mu.Unlock()
		return nil
	}
	if err := pc.AddICECandidate(cand); err != nil {
		log.Println("viewer AddICECandidate error:", err)
	}
	return nil
}

func (s *viewerSession) writeSignal(msg signalMessage) error {
	s.mu.Lock()
	ws := s.ws
	s.mu.Unlock()
	if ws == nil {
		return errors.New("信令连接不存在")
	}
	return ws.WriteJSON(msg)
}

func (s *viewerSession) stop() {
	s.cancel()

	s.mu.Lock()
	if s.dc != nil {
		s.dc.Close()
		s.dc = nil
	}
	if s.pc != nil {
		s.pc.Close()
		s.pc = nil
	}
	if s.ws != nil {
		s.ws.Close()
		s.ws = nil
	}
	s.mu.Unlock()
}


