package server

// PublisherStream 保存 Publisher 和其 Viewer 列表
type PublisherStream struct {
	Publisher *Client
	Viewers   map[string]*Client // PeerID -> Client
}

func NewPublisherStream(pub *Client) *PublisherStream {
	return &PublisherStream{
		Publisher: pub,
		Viewers:   make(map[string]*Client),
	}
}

func (s *PublisherStream) AddViewer(peerID string, c *Client) {
	s.Viewers[peerID] = c
}

func (s *PublisherStream) RemoveViewer(peerID string) {
	delete(s.Viewers, peerID)
}
