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
			if got < ceiling || got > ceiling+ceiling/2 {
				t.Fatalf("defaultBackoff(%d) = %v, want within [%v, %v]", attempt, got, ceiling, ceiling+ceiling/2)
			}
		}
	}

	// Even with no jitter at all the waits must cover a per-minute rate-limit window, and even with
	// maximum jitter they must fit the retry budget.
	var shortest, longest time.Duration
	for a := 1; a < MaxAttempts; a++ {
		base := min(time.Duration(1)<<(a-1)*time.Second, maxBackoff)
		shortest += base
		longest += base + base/2
	}
	if shortest < time.Minute {
		t.Errorf("shortest total wait %v does not outlast a one-minute rate-limit window", shortest)
	}
	if longest > defaultRetryBudget {
		t.Errorf("longest total wait %v exceeds the %v retry budget", longest, defaultRetryBudget)
	}
}
