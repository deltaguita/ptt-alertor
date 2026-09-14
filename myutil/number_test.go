package myutil

import "testing"

func TestComma(t *testing.T) {
	for _, tt := range []struct {
		in   int
		want string
	}{
		{0, "0"}, {999, "999"}, {1000, "1,000"}, {36500, "36,500"},
		{1234567, "1,234,567"}, {-4200, "-4,200"},
	} {
		if got := Comma(tt.in); got != tt.want {
			t.Errorf("Comma(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
