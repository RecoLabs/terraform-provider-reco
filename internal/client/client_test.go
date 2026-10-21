package client

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestScimQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"simple", `"simple"`},
		{`with"quote`, `"with\"quote"`},
		{`with\slash`, `"with\\slash"`},
		{`"both`, `"\"both"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := scimQuote(tc.in); got != tc.want {
			t.Errorf("scimQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	nfe := &NotFoundError{Message: "not found"}
	if !IsNotFound(nfe) {
		t.Fatal("expected IsNotFound true for *NotFoundError")
	}
	if !IsNotFound(fmt.Errorf("wrap: %w", nfe)) {
		t.Fatal("expected IsNotFound true for wrapped *NotFoundError")
	}
	if IsNotFound(fmt.Errorf("unrelated error")) {
		t.Fatal("expected IsNotFound false for unrelated error")
	}
	if IsNotFound(nil) {
		t.Fatal("expected IsNotFound false for nil")
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{" 5 ", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"garbage", 0},
		{"99999", maxRetryAfter},
		{now.Add(7 * time.Second).Format(http.TimeFormat), 7 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{now.Add(24 * time.Hour).Format(http.TimeFormat), maxRetryAfter},
	}
	for _, tt := range tests {
		h := http.Header{}
		if tt.header != "" {
			h.Set("Retry-After", tt.header)
		}
		if got := retryAfter(h, now); got != tt.want {
			t.Errorf("retryAfter(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestDefaultBackoff(t *testing.T) {
	for attempt := 1; attempt <= 12; attempt++ {
		ceiling := min(time.Duration(1)<<(attempt-1)*time.Second, maxBackoff)
		for range 50 {
			got := defaultBackoff(attempt)
			if got > ceiling || got < ceiling/2 {
				t.Fatalf("defaultBackoff(%d) = %v, want within [%v, %v]", attempt, got, ceiling/2, ceiling)
			}
		}
	}
	if total := func() (sum time.Duration) {
		for a := 1; a < MaxAttempts; a++ {
			sum += min(time.Duration(1)<<(a-1)*time.Second, maxBackoff)
		}
		return sum
	}(); total < 45*time.Second {
		t.Errorf("worst-case total wait %v is too short to outlast a rate-limit window", total)
	}
}
