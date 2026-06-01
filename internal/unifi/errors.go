package unifi

import "fmt"

// CapabilityError is returned when the connected firmware lacks a capability a
// requested operation needs. Resources surface it as a plan-time diagnostic via
// ModifyPlan/ValidateConfig — never as a raw 4xx mid-apply (development.md
// invariant 3).
type CapabilityError struct {
	Capability string
	Detail     string
}

func (e *CapabilityError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("capability %q unavailable: %s", e.Capability, e.Detail)
	}
	return fmt.Sprintf("capability %q unavailable on this firmware", e.Capability)
}

// ConfigError is a provider-configuration problem detected before any API call
// (missing host or credentials).
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }
