package e911

import (
	"context"
	"errors"
)

var (
	ErrUnsupportedProvider     = errors.New("unsupported provider")
	ErrChallengeNotImplemented = errors.New("challenge not implemented")
	ErrWebsheetUnavailable     = errors.New("websheet unavailable")
)

type Identity struct {
	IMSI        string
	IMEI        string
	MCC         string
	MNC         string
	SIPUsername string
	DisplayName string
}

type HeaderPair struct {
	Key   string
	Value string
}

type HTTPRequest struct {
	Method  string
	URL     string
	Headers []HeaderPair
	Body    []byte
}

type HTTPResponse struct {
	StatusCode int
	Headers    []HeaderPair
	Body       []byte
}

type HTTPClient interface {
	Do(req *HTTPRequest) (*HTTPResponse, error)
}

type defaultHTTPClient struct{}

func (c *defaultHTTPClient) Do(req *HTTPRequest) (*HTTPResponse, error) {
	return &HTTPResponse{StatusCode: 200}, nil
}

func NewDefaultHTTPClient() HTTPClient {
	return &defaultHTTPClient{}
}

type Request struct {
	Carrier     any
	Identity    Identity
	AKAProvider any
	Client      HTTPClient
	Trace       any
}

type WebsheetRequest struct {
	URL         string
	UserData    string
	ContentType string
	Title       string
}

func StartEmergencyAddressUpdate(ctx context.Context, req Request) (WebsheetRequest, error) {
	return WebsheetRequest{}, ErrUnsupportedProvider
}
