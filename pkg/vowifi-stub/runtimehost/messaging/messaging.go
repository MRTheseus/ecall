package messaging

import (
	"context"
	"errors"
	"time"
)

var ErrDeliveryNotFound = errors.New("delivery not found")

func WithSuppressSendTGSuccess(ctx context.Context) context.Context {
	return ctx
}

type DeliveryStore interface{}

type DeliveryPartMatch struct {
	MessageID string
	PartNo    int
	State     string
}

type DeliveryStatus struct {
	MessageID  string
	IMSI       string
	DeviceID   string
	Peer       string
	Content    string
	PartsTotal int
	Acks       int
	State      string
	LastError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Parts      []DeliveryPartStatus
}

type DeliveryPartStatus struct {
	PartNo      int
	CallID      string
	InReplyTo   string
	RPMR        int
	State       string
	SIPCode     int
	RPCause     int
	RPCauseText string
	ErrorText   string
	SentAt      time.Time
	ReportAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func RPCauseText(cause int) string {
	return ""
}

type SendOutcome struct {
	PartsTotal    int
	DeliveryState string
	MessageID     string
}

type SendOptions struct {
	Encoding string
}

type USSDResult struct {
	Message      string
	SessionEnded bool
}
