package service

import "testing"

func TestValidRate(t *testing.T) {
	for _, ok := range []string{"20M", "10M", "512K", "1G", "100", "999999"} {
		if !validRate(ok) {
			t.Errorf("validRate(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "20Mbps", "M20", "-5M", "20 M", "1234567890M"} {
		if validRate(bad) {
			t.Errorf("validRate(%q) = true, want false", bad)
		}
	}
}

func TestMACNormalization(t *testing.T) {
	if got := normalizeMAC("3E-A8-32-EB-45-16"); got != "3ea832eb4516" {
		t.Errorf("normalizeMAC dash: %q", got)
	}
	if got := colonMAC("3e-a8-32-eb-45-16"); got != "3E:A8:32:EB:45:16" {
		t.Errorf("colonMAC: %q", got)
	}
	if got := colonMAC("short"); got != "" {
		t.Errorf("colonMAC short: %q", got)
	}
}

func TestStripCIDR(t *testing.T) {
	if got := stripCIDR("172.16.88.112/32"); got != "172.16.88.112" {
		t.Errorf("stripCIDR: %q", got)
	}
	if got := stripCIDR("192.168.1.5"); got != "192.168.1.5" {
		t.Errorf("stripCIDR plain: %q", got)
	}
}

func TestQueueNameForIP(t *testing.T) {
	if got := queueNameForIP("192.168.255.254/32"); got != "airlink-192-168-255-254" {
		t.Errorf("queueNameForIP: %q", got)
	}
}
