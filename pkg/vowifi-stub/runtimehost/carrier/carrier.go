package carrier

import "fmt"

type LoadOverridesResult struct {
	Path    string
	Count   int
	Missing bool
}

func LoadCarrierOverrides(path string) (LoadOverridesResult, error) {
	return LoadOverridesResult{Missing: true}, nil
}

func ClearCarrierOverrides() {}

func IsVoWiFiBlockedMCC(mcc string) bool {
	return mcc == "460"
}

func NewVoWiFiBlockedMCCError(mcc string) error {
	return fmt.Errorf("VoWiFi blocked for MCC %s", mcc)
}

func IsVoWiFiPolicyBlockedError(err error) bool {
	return err != nil
}

type EffectiveCarrierConfigInput struct {
	MCC string
	MNC string
}

type E911Config struct {
	Enabled  bool
	Provider string
}

type EffectiveCarrierConfig struct {
	WebsheetURL string
	E911        E911Config
}

func ResolveEffectiveCarrierConfig(in EffectiveCarrierConfigInput) EffectiveCarrierConfig {
	return EffectiveCarrierConfig{}
}
