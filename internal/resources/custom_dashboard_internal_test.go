package resources

import "testing"

func TestJSONEquivalent(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", `{"a":1}`, `{"a":1}`, true},
		{"whitespace", `{"a": 1, "b": [1, 2]}`, `{"b":[1,2],"a":1}`, true},
		{"different values", `{"a":1}`, `{"a":2}`, false},
		{"different array order", `[1,2]`, `[2,1]`, false},
		{"invalid equal", `nope`, `nope`, true},
		{"invalid different", `nope`, `{}`, false},
	}
	for _, tc := range tests {
		if got := jsonEquivalent(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: jsonEquivalent(%q, %q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}
