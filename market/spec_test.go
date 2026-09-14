package market

import "testing"

func TestParseSpec(t *testing.T) {
	tests := []struct {
		in   string
		want Spec
	}{
		{"iPhone 17 Pro Max 256", Spec{"17", VariantProMax, 256}},
		{"17 pro max 256g", Spec{"17", VariantProMax, 256}},
		{"17 ProMax 512GB", Spec{"17", VariantProMax, 512}},
		{"iPhone 17 Pro 256", Spec{"17", VariantPro, 256}},
		{"16 Pro 1TB", Spec{"16", VariantPro, 1024}},
		{"15 Plus 128", Spec{"15", VariantPlus, 128}},
		{"17 Pro Max", Spec{"17", VariantProMax, 0}},
		{"17", Spec{"17", "", 0}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseSpec(tt.in); got != tt.want {
				t.Errorf("ParseSpec(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSpecString(t *testing.T) {
	for _, tt := range []struct {
		spec Spec
		want string
	}{
		{Spec{"17", VariantProMax, 256}, "iPhone 17 Pro Max 256G"},
		{Spec{"16", VariantPro, 1024}, "iPhone 16 Pro 1TB"},
		{Spec{"17", "", 0}, "iPhone 17"},
	} {
		if got := tt.spec.String(); got != tt.want {
			t.Errorf("Spec.String() = %q, want %q", got, tt.want)
		}
	}
}
