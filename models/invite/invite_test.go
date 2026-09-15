package invite

import (
	"strings"
	"testing"
)

func TestGenerateAvoidsAmbiguousCharacters(t *testing.T) {
	// A code gets copied by hand or read aloud, so the pairs that are misread --
	// 0/O and 1/I/l -- must never appear.
	for i := 0; i < 200; i++ {
		code := generate()
		if len(code) != codeLength {
			t.Fatalf("code %q is %d characters, want %d", code, len(code), codeLength)
		}
		if strings.ContainsAny(code, "01OIL") {
			t.Errorf("code %q contains an ambiguous character", code)
		}
	}
}

func TestGenerateDoesNotRepeatItself(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 500; i++ {
		code := generate()
		if seen[code] {
			t.Fatalf("code %q was issued twice in 500 draws", code)
		}
		seen[code] = true
	}
}

func TestLooks(t *testing.T) {
	code := generate()
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{code, true},
		{strings.ToLower(code), true}, // typed in lower case
		{" " + code + " ", true},      // pasted with whitespace
		{"行情 iPhone 17 Pro", false},
		{"ABCDEFG", false},   // too short
		{"ABCDEFGHI", false}, // too long
		{"ABCDEF0O", false},  // characters outside the alphabet
		{"", false},
	} {
		if got := Looks(tt.in); got != tt.want {
			t.Errorf("Looks(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRedeemedReportsUse(t *testing.T) {
	if (Code{}).Redeemed() {
		t.Error("a fresh code reports itself as used")
	}
	if !(Code{RedeemedBy: "123"}).Redeemed() {
		t.Error("a used code reports itself as fresh")
	}
}
