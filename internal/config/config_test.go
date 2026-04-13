package config

import "testing"

func TestValidateRelayRequiresIPPortWhenEnabled(t *testing.T) {
	cfg := Default()
	cfg.UseRelay = true

	for _, relayAddr := range []string{"", "localhost:228", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:70000"} {
		t.Run(relayAddr, func(t *testing.T) {
			cfg.RelayAddr = relayAddr
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() succeeded for relay_addr=%q", relayAddr)
			}
		})
	}

	cfg.RelayAddr = "127.0.0.1:228"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateIgnoresRelayAddrWhenDisabled(t *testing.T) {
	cfg := Default()
	cfg.UseRelay = false
	cfg.RelayAddr = "not ip:port"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
