package voicecall

import (
	"time"

	"github.com/pion/webrtc/v4"
)

// CallState 表示通话状态
type CallState string

const (
	CallStateIdle       CallState = "idle"       // 空闲
	CallStateDialing    CallState = "dialing"    // 正在拨号
	CallStateRinging    CallState = "ringing"    // 对方振铃 / 本地来电振铃
	CallStateActive     CallState = "active"     // 通话中
	CallStateTerminated CallState = "terminated" // 通话结束
)

// CallDirection 呼叫方向
type CallDirection string

const (
	DirectionOutbound CallDirection = "outbound" // 呼出
	DirectionInbound  CallDirection = "inbound"  // 呼入
)

// CallSession 表示一次通话会话信息
type CallSession struct {
	ID           string        `json:"id"`
	DeviceID     string        `json:"device_id"`
	RemoteNumber string        `json:"remote_number"`
	Direction    CallDirection `json:"direction"`
	State        CallState     `json:"state"`
	IsRecording   bool          `json:"is_recording"`
	RecordingFile string        `json:"recording_file,omitempty"`
	RecordingSize int64         `json:"recording_size,omitempty"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	ConnectedAt   *time.Time    `json:"connected_at,omitempty"`
	EndedAt       *time.Time    `json:"ended_at,omitempty"`
	DurationSec   int64         `json:"duration_sec"`
	HangupReason  string        `json:"hangup_reason,omitempty"`
}

// CallEvent 实时推送给前端的通话状态变更事件
type CallEvent struct {
	Type      string       `json:"type"` // "state_change", "incoming", "hangup", "audio_level"
	Session   *CallSession `json:"session,omitempty"`
	Timestamp int64        `json:"timestamp"`
	Message   string       `json:"message,omitempty"`
}

// DialRequest 拨号请求参数
type DialRequest struct {
	DeviceID string `json:"device_id"`
	Number   string `json:"number"`
}

// DTMFRequest 发送按键音请求
type DTMFRequest struct {
	Digit string `json:"digit"`
}

// WebRTCOfferRequest WebRTC SDP Offer 请求
type WebRTCOfferRequest struct {
	SDP string `json:"sdp"`
}

// WebRTCAnswerResponse WebRTC SDP Answer 响应
type WebRTCAnswerResponse struct {
	SDP string `json:"sdp"`
}

// WebRTCCandidateRequest WebRTC ICE Candidate 请求
type WebRTCCandidateRequest struct {
	Candidate     string  `json:"candidate"`
	SDPMid        string  `json:"sdpMid"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
}

func webrtcCandidateInit(req WebRTCCandidateRequest) webrtc.ICECandidateInit {
	return webrtc.ICECandidateInit{
		Candidate:     req.Candidate,
		SDPMid:        &req.SDPMid,
		SDPMLineIndex: req.SDPMLineIndex,
	}
}
