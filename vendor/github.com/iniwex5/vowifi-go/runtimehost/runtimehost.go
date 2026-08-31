package runtimehost

import (
	"context"
	"errors"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
	"go.uber.org/zap"
)

var ErrAPDUBusy = errors.New("apdu busy")

const (
	StartModeMain  = "main"
	PhaseSIMReady  = "SIMReady"
)

type State struct {
	DeviceID       string
	SIMReady       bool
	AccessReady    bool
	TunnelReady    bool
	IMSReady       bool
	SMSReady       bool
	Phase          string
	NetworkMode    string
	DataplaneMode  string
	LastReason     string
	LastErrorClass string
	LastError      string
	RegStatus      int
	RegStatusText  string
	UpdatedAt      time.Time
}

type ProxyConfig struct {
	ID       string
	Addr     string
	Username string
	Password string
	Enabled  bool
	Server   string
}

type ServiceStub struct{}

func (s *ServiceStub) SendSMSWithOptions(ctx context.Context, to, text string, opts messaging.SendOptions) (messaging.SendOutcome, error) {
	return messaging.SendOutcome{}, nil
}
func (s *ServiceStub) SendUSSD(ctx context.Context, command string) (*messaging.USSDResult, error) {
	return &messaging.USSDResult{}, nil
}
func (s *ServiceStub) ContinueUSSD(ctx context.Context, sessionID, input string) (*messaging.USSDResult, error) {
	return &messaging.USSDResult{}, nil
}
func (s *ServiceStub) CancelUSSD(ctx context.Context, sessionID string) error {
	return nil
}

type Instance struct{}

func (i *Instance) AddObserver(o Observer)                 {}
func (i *Instance) Stop(ctx context.Context) error         { return nil }
func (i *Instance) State() State                          { return State{} }
func (i *Instance) SetNotifier(n any)                     {}
func (i *Instance) SetSMSNotifier(n any)                  {}
func (i *Instance) TriggerMOBIKE(args ...string) error    { return nil }
func (i *Instance) Service() *ServiceStub                 { return &ServiceStub{} }
func (i *Instance) Status() string                        { return "idle" }
func (i *Instance) Obs() map[string]interface{}           { return nil }
func (i *Instance) GetSMSDeliveryStatus(messageID string) (*messaging.DeliveryStatus, error) {
	return nil, nil
}

type Observer interface{}
type ObserverFunc func(context.Context, Event)

type Event struct {
	State State
}

type StartRequest struct {
	Mode          string
	DeviceID      string
	TraceID       string
	Profile       Profile
	Prepared      any
	NetworkMode   string
	VoiceGateway  any
	SIM           SIMAdapter
	Access        any
	Dataplane     DataplanePolicy
	Proxy         any
	DeliveryStore any
	Dispatch      any
	BeforeStart   func(context.Context, SessionConfig) error
	ShouldRun     func() bool
}

type Profile struct {
	IMSI string
	MCC  string
	MNC  string
	IMEI string
	SMSC string
}

type SessionConfig struct {
	DataplaneMode string
}

type DataplanePolicy struct {
	Mode string
}

type SIMAdapter interface{}

type Modem interface {
	GetNetworkMode() string
	GetRegStatus() (int, string)
}

type ReaderSIMAdapter struct {
	Provider any
}

func NewReaderSIMAdapter(provider any) SIMAdapter {
	return &ReaderSIMAdapter{Provider: provider}
}

type ModemAccessAdapter struct {
	Modem Modem
}

func NewModemAccessAdapter(modem Modem) any {
	return &ModemAccessAdapter{Modem: modem}
}

func SetLogger(l *zap.Logger) {}

func NewTraceID() string {
	return "trace"
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return ctx
}

func Start(ctx context.Context, req StartRequest) (*Instance, error) {
	return &Instance{}, nil
}
