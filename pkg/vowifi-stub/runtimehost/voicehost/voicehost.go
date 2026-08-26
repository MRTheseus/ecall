package voicehost

import (
	"context"
)

const (
	DefaultSimulateCallHoldSeconds = 15
	MaxSimulateCallHoldSeconds     = 60
)

type Gateway struct{}

func NewGateway() *Gateway {
	return &Gateway{}
}

func (g *Gateway) Start(ctx context.Context) error { return nil }
func (g *Gateway) Stop() error                     { return nil }
func (g *Gateway) SetNotifier(n any)               {}
func (g *Gateway) DeviceStatus(deviceID string) any {
	return map[string]any{"status": "idle"}
}
func (g *Gateway) GetAgent(deviceID string) any { return nil }

type SimulateCallRequest struct {
	Callee      string
	HoldSeconds int
	OnConnected func()
}

type SimulateCallResponse struct {
	Success    bool
	DurationMs int64
	Reason     string
}

func (g *Gateway) SimulateCall(ctx context.Context, deviceID string, req SimulateCallRequest) (SimulateCallResponse, error) {
	return SimulateCallResponse{Success: true, DurationMs: 1000}, nil
}
