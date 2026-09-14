package iphone

import (
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/market"
	"github.com/Ptt-Alertor/ptt-alertor/price"
)

// kind is a value rather than a literal at each call site: Go cannot tell a
// composite literal from a block when one opens an if statement.
var kind = Kind{}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		in   string
		want market.Attrs
	}{
		{"iPhone 17 Pro Max 256", market.Attrs{"model": "17", "variant": "Pro Max", "capacity": "256"}},
		{"17 pro max 256g", market.Attrs{"model": "17", "variant": "Pro Max", "capacity": "256"}},
		// No space between Pro and Max, as sellers really write it.
		{"17 ProMax 512GB", market.Attrs{"model": "17", "variant": "Pro Max", "capacity": "512"}},
		{"iPhone 17 Pro 256", market.Attrs{"model": "17", "variant": "Pro", "capacity": "256"}},
		{"16 Pro 1TB", market.Attrs{"model": "16", "variant": "Pro", "capacity": "1024"}},
		{"15 Plus 128", market.Attrs{"model": "15", "variant": "Plus", "capacity": "128"}},
		{"17 Pro Max", market.Attrs{"model": "17", "variant": "Pro Max"}},
		{"17", market.Attrs{"model": "17"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := kind.ParseQuery(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseQuery(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for name, want := range tt.want {
				if got.Get(name) != want {
					t.Errorf("ParseQuery(%q)[%s] = %q, want %q", tt.in, name, got.Get(name), want)
				}
			}
		})
	}
}

func TestParseQueryRejectsNonPhones(t *testing.T) {
	for _, text := range []string{"AirPods Pro", "Mac mini M4", ""} {
		if got := kind.ParseQuery(text); got != nil {
			t.Errorf("ParseQuery(%q) = %v, want nil so another kind can claim it", text, got)
		}
	}
}

func TestTitlePattern(t *testing.T) {
	pattern := kind.TitlePattern()
	for _, title := range []string{
		"[販售] 台北 iPhone 17 Pro Max 256G 橘色",
		"[販售] 新竹 iphone 16 pro 128g",
		"[販售] 台中 iPhone 19 Pro", // a generation that does not exist yet
	} {
		if !pattern.MatchString(title) {
			t.Errorf("pattern missed %q", title)
		}
	}
	for _, title := range []string{"[販售] 全國 AirPods 4 (ANC)", "[販售] 桃園 Mac mini M4 16/512"} {
		if pattern.MatchString(title) {
			t.Errorf("pattern matched non-phone %q", title)
		}
	}
}

func TestAttrsRequiresCapacityToKeepAccessoriesOut(t *testing.T) {
	// "[販售] 全國 iPhone 16 原廠矽膠保護殼" at 699 matched the title pattern and
	// was extracted as an iPhone 16 -- three colours made three false records.
	// A case has no capacity, and a genuine handset listing always states one.
	accessory := price.Item{Name: "iPhone 16 原廠矽膠保護殼", Price: 699,
		Attrs: map[string]string{"model": "16"}}
	if attrs, ok := kind.Attrs(accessory); ok {
		t.Errorf("accessory accepted as a handset: %v", attrs)
	}

	handset := price.Item{Name: "iPhone 17 Pro Max 256G", Price: 36000,
		Attrs: map[string]string{"model": "17", "variant": "Pro Max", "capacity": "256", "battery": "92"}}
	attrs, ok := kind.Attrs(handset)
	if !ok {
		t.Fatal("handset rejected")
	}
	if attrs.Get("capacity") != "256" || attrs.Get("battery") != "92" {
		t.Errorf("Attrs() = %v", attrs)
	}
}

func TestLabel(t *testing.T) {
	for _, tt := range []struct {
		attrs market.Attrs
		want  string
	}{
		{market.Attrs{"model": "17", "variant": "Pro Max", "capacity": "256"}, "iPhone 17 Pro Max 256G"},
		{market.Attrs{"model": "16", "variant": "Pro", "capacity": "1024"}, "iPhone 16 Pro 1TB"},
		{market.Attrs{"model": "17"}, "iPhone 17"},
	} {
		if got := kind.Label(tt.attrs); got != tt.want {
			t.Errorf("Label(%v) = %q, want %q", tt.attrs, got, tt.want)
		}
	}
}

func TestDedupeKeyNeedsCapacity(t *testing.T) {
	if _, ok := kind.DedupeKey(market.Attrs{"model": "17", "variant": "Pro"}); ok {
		t.Error("DedupeKey() accepted attributes too coarse to identify one handset")
	}
	key, ok := kind.DedupeKey(market.Attrs{"model": "17", "variant": "Pro", "capacity": "256"})
	if !ok || key == "" {
		t.Errorf("DedupeKey() = (%q, %v), want a key", key, ok)
	}
}

func TestRegistered(t *testing.T) {
	registered, ok := market.KindByName("iphone")
	if !ok {
		t.Fatal("iphone kind did not register itself")
	}
	if registered.Name() != "iphone" {
		t.Errorf("registered as %q", registered.Name())
	}
}

func TestAttrsFillsGapsFromTheItemName(t *testing.T) {
	// What the extractor really returns: it judges the variant well but often
	// omits the capacity that is sitting in the name.
	item := price.Item{
		Name:  "IPHONE 17 PRO MAX 256G",
		Price: 33500,
		Attrs: map[string]string{"model": "17", "variant": "Pro Max"},
	}
	attrs, ok := kind.Attrs(item)
	if !ok {
		t.Fatal("handset rejected for a capacity that is in its name")
	}
	if attrs.Get(AttrCapacity) != "256" {
		t.Errorf("capacity = %q, want 256 read from the name", attrs.Get(AttrCapacity))
	}
}

func TestAttrsPrefersTheExtractorOverTheName(t *testing.T) {
	// "pm" is Pro Max to a reader and nothing to a pattern, so the extractor's
	// answer has to win.
	item := price.Item{
		Name:  "iphone 17 pm 256 白色",
		Price: 31000,
		Attrs: map[string]string{"model": "17", "variant": "Pro Max"},
	}
	attrs, ok := kind.Attrs(item)
	if !ok {
		t.Fatal("handset rejected")
	}
	if attrs.Get(AttrVariant) != VariantProMax {
		t.Errorf("variant = %q, want Pro Max from the extractor", attrs.Get(AttrVariant))
	}
}

func TestAttrsRejectsAnItemTheExtractorDidNotClaim(t *testing.T) {
	// Empty attrs is how the extractor says "not a handset". The name alone must
	// not be enough to override that, or every accessory comes back.
	item := price.Item{Name: "全新 iPhone 16 256G 原廠矽膠殼", Price: 699}
	if attrs, ok := kind.Attrs(item); ok {
		t.Errorf("accessory accepted from its name alone: %v", attrs)
	}
}

func TestNormaliseCapacity(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"256", "256"}, {"256G", "256"}, {"512 GB", "512"},
		{"1TB", "1024"}, {"1024", "1024"}, {"", ""}, {"0", ""}, {"未提及", ""},
	} {
		if got := normaliseCapacity(tt.in); got != tt.want {
			t.Errorf("normaliseCapacity(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormaliseBattery(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"92", "92"}, {"100%", "100"}, {"", ""}, {"0", ""}, {"9000", ""}, {"高", ""},
	} {
		if got := normaliseBattery(tt.in); got != tt.want {
			t.Errorf("normaliseBattery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAttrLabelAndValueDoNotLeakTheSchema(t *testing.T) {
	if got := kind.AttrLabel(AttrCapacity); got != "容量" {
		t.Errorf("AttrLabel(capacity) = %q, want 容量", got)
	}
	for _, tt := range []struct{ name, value, want string }{
		{AttrCapacity, "512", "512G"},
		{AttrCapacity, "1024", "1TB"},
		{AttrBattery, "92", "92%"},
		{AttrModel, "17", "17"},
	} {
		if got := kind.AttrValue(tt.name, tt.value); got != tt.want {
			t.Errorf("AttrValue(%s, %s) = %q, want %q", tt.name, tt.value, got, tt.want)
		}
	}
}
