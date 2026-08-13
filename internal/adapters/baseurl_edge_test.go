package adapters

import (
	"strings"
	"testing"
)

func TestValidateBaseURLMatrix(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain https", "https://api.example.com", ""},
		{"loopback allowed", "http://127.0.0.1:8080", ""},
		{"ipv6 loopback allowed", "http://[::1]:8080", ""},
		{"link-local refused", "http://169.254.169.254/latest", "link-local"},
		{"unspecified refused", "http://0.0.0.0:8080", "real address"},
		{"ipv6 unspecified refused", "http://[::]:8080", "real address"},
		{"ipv6 link-local refused", "http://[fe80::1]:8080", "link-local"},
		{"scheme refused", "ftp://x", "scheme"},
		{"credentials refused", "https://user:pass@x", "credentials"},
		{"no host", "https:///path", "host"},
		{"empty refused", "   ", "base_url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURL(tc.raw)
			if tc.want == "" {
				if err != nil {
					t.Errorf("must be accepted, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error must name %q, got %v", tc.want, err)
			}
		})
	}
}
