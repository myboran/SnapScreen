package signal

// MessageType 是客户端 / 服务器之间信令消息中的 type 字段取值
type MessageType string

const (
	MsgTypeRegister     MessageType = "register"
	MsgTypeUnregister   MessageType = "unregister"
	MsgTypeListStreams  MessageType = "list_streams"
	MsgTypeSubscribe    MessageType = "subscribe"
	MsgTypeUnsubscribe  MessageType = "unsubscribe"
	MsgTypeOffer        MessageType = "offer"
	MsgTypeAnswer       MessageType = "answer"
	MsgTypeICECandidate MessageType = "ice_candidate"
	MsgTypeStreamList   MessageType = "stream_list"
	MsgTypeError        MessageType = "error"
	MsgTypeSuccess      MessageType = "success"
)

type Message struct {
	Type     MessageType `json:"type"`
	StreamID string      `json:"stream_id,omitempty"`
	PeerID   string      `json:"peer_id,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Error    string      `json:"error,omitempty"`
}
