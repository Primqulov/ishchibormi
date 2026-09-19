package config

import "testing"

func TestWebBaseURL(t *testing.T) {
	for _, tc := range []struct {
		raw               string
		production, valid bool
	}{
		{"https://ishchibormi.uz", true, true},
		{"http://127.0.0.1:3000", false, true},
		{"http://localhost:3000", false, false},
		{"http://LOCALHOST:3000", false, false},
		{"http://localhost.:3000", false, false},
		{"http://web:3000", false, false},
		{"http://localhost:3000", true, false},
		{"javascript:alert(1)", false, false},
		{"https://user:password@example.com", false, false},
		{"https://example.com/path", false, false},
		{"https://example.com?secret=value", false, false},
		{"https://example.com#fragment", false, false},
		{"", false, false},
	} {
		if valid := webBaseURLProblem(tc.raw, tc.production) == ""; valid != tc.valid {
			t.Errorf("URL %q production=%v valid=%v, want %v", tc.raw, tc.production, valid, tc.valid)
		}
	}
}
