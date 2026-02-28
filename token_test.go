package main

import "testing"

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-ant-1234567890abcdef", "sk-a***************cdef"}, // 23 chars: first4 + 15 asterisks + last4
		{"abcdefgh", "********"},                                // exactly 8: all masked
		{"abcdefghi", "abcd*fghi"},                              // 9 chars: first4 + 1 asterisk + last4
		{"short", "*****"},               // <8: all masked
		{"a", "*"},                       // single char
		{"ab", "**"},                     // 2 chars
		{"abcdefghijklmnop", "abcd********mnop"}, // 16 chars
	}

	for _, tt := range tests {
		got := maskToken(tt.input)
		if got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
