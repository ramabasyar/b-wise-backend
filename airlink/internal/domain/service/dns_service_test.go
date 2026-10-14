package service

import "testing"

func TestValidateDNSRecord(t *testing.T) {
	tests := []struct {
		name   string
		record DNSRecord
		valid  bool
	}{
		{"internal A", DNSRecord{Name: "host.binawan.ac.id", Type: "A", Content: "10.123.120.25", TTL: 300}, true},
		{"public zone root blocked", DNSRecord{Name: "binawan.ac.id", Type: "A", Content: "10.123.120.25", TTL: 300}, false},
		{"external zone blocked", DNSRecord{Name: "google.com", Type: "A", Content: "10.123.120.25", TTL: 300}, false},
		{"bad IPv4", DNSRecord{Name: "host.binawan.ac.id", Type: "A", Content: "not-an-ip", TTL: 300}, false},
		{"SOA protected", DNSRecord{Name: "host.binawan.ac.id", Type: "SOA", Content: "x", TTL: 300}, false},
		{"CNAME target", DNSRecord{Name: "alias.binawan.ac.id", Type: "CNAME", Content: "host.binawan.ac.id", TTL: 300}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDNSRecord(&tc.record)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
