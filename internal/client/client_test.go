package client

import (
	"fmt"
	"testing"
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
