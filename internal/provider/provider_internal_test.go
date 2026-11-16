package provider

import "testing"

func TestIsSecureBaseURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://acme.reco.ai", true},
		{"https://acme.reco.ai/", true},
		{"http://127.0.0.1:8080", true},
		{"http://localhost:8080", true},
		{"http://[::1]:8080", true},
		{"http://acme.reco.ai", false},
		{"http://example.com", false},
		{"http://localhost.evil.com", false},
		{"http://127.0.0.1.evil.com", false},
		{"http://10.0.0.1", false},
		{"ftp://localhost", false},
		{"localhost:8080", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isSecureBaseURL(tt.url); got != tt.want {
				t.Errorf("isSecureBaseURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestClientOptions_paceOnlyRealHosts(t *testing.T) {
	tests := []struct {
		url       string
		wantPaced bool
	}{
		{"https://us04.us.reco.ai", true},
		{"https://acme.reco.ai/", true},
		{"https://localhost.evil.com", true},
		{"http://127.0.0.1:8080", false},
		{"http://localhost:8080", false},
		{"http://[::1]:8080", false},
	}
	for _, tt := range tests {
		if got := len(clientOptions(tt.url)) > 0; got != tt.wantPaced {
			t.Errorf("clientOptions(%q) paced = %v, want %v", tt.url, got, tt.wantPaced)
		}
	}
}
