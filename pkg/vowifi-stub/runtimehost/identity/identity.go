package identity

import "github.com/iniwex5/vowifi-go/runtimehost"

type Profile = runtimehost.Profile

type Identity struct {
	IMSI   string
	MSISDN string
	MCC    string
	MNC    string
}

type EffectiveCarrier struct {
	MCC      string
	MNC      string
	PresetID string
}

type IMSIdentity struct {
	RequestedSource  string
	ActualSource     string
	AKAAppPreference string
	Applied          bool
}

type PreparedSession struct {
	Profile           runtimehost.Profile
	EffectiveCarrier  EffectiveCarrier
	EPDGSource        string
	EPDGAddr          string
	IdentityIMEISource string
	IMSIdentity       IMSIdentity
}

type PrepareStartInput struct {
	DeviceID            string
	Profile             runtimehost.Profile
	RuntimeEPDGOverride string
	Access              any
}

func PrepareStart(in PrepareStartInput) (PreparedSession, error) {
	return PreparedSession{Profile: in.Profile}, nil
}

func ReadISIMIdentity(adapter any) (Identity, error) {
	return Identity{}, nil
}
