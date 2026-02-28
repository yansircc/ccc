package main

import "testing"

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		a, b string
		want int // >0 means a>b, <0 means a<b, 0 means equal
	}{
		{"1.2.3", "1.2.4", -1},
		{"1.2.4", "1.2.3", 1},
		{"2.0", "1.9.9", 1},
		{"1.9.9", "2.0", -1},
		{"1.0", "1.0", 0},
		{"1.0.0", "1.0", 1},   // numerically equal, but strings.Compare tiebreak: "1.0.0" > "1.0"
		{"v1.2.3", "1.2.3", 1}, // numerically equal, but strings.Compare tiebreak: "v" > "1"
		{"v2.0.0", "v1.9.9", 1},
		{"0.1", "0.2", -1},
		{"10.0", "9.99", 1},
		{"1.0.0.1", "1.0.0", 1},
	}

	for _, tt := range tests {
		got := compareVersion(tt.a, tt.b)
		if (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) || (tt.want == 0 && got != 0) {
			t.Errorf("compareVersion(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseVersionPart(t *testing.T) {
	tests := []struct {
		parts []string
		idx   int
		want  int
	}{
		{[]string{"1", "2", "3"}, 0, 1},
		{[]string{"1", "2", "3"}, 2, 3},
		{[]string{"1", "2", "3"}, 5, 0},  // out of bounds
		{[]string{"abc"}, 0, 0},           // non-numeric
		{[]string{"42"}, 0, 42},           // valid int
		{[]string{}, 0, 0},                // empty parts
	}

	for _, tt := range tests {
		got := parseVersionPart(tt.parts, tt.idx)
		if got != tt.want {
			t.Errorf("parseVersionPart(%v, %d) = %d, want %d", tt.parts, tt.idx, got, tt.want)
		}
	}
}
