package sim

import "errors"

var ErrSyncFailure = errors.New("aka sync failure")

type AKAResult struct {
	RES  []byte
	CK   []byte
	IK   []byte
	AUTS []byte
}

type AKAProvider interface {
	CalculateAKA(rand, autn []byte) (AKAResult, error)
}
