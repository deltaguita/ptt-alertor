// Package iphone tracks second-hand iPhone listings.
//
// Everything specific to phones lives here: which listings are worth reading,
// what the extractor is asked to pull out of one, how a typed query is
// understood, and what makes two listings the same handset. The market package
// supplies the rest -- storage, scanning, budgeting and statistics -- and knows
// nothing about phones.
package iphone

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/market"
	"github.com/Ptt-Alertor/ptt-alertor/price"
)

// Attribute names this kind records.
const (
	AttrModel    = "model"
	AttrVariant  = "variant"
	AttrCapacity = "capacity"
	AttrBattery  = "battery"
)

// Variants a handset may be. Order matters when classifying from text:
// "17 Pro Max" contains "Pro", so Pro Max has to be tested first.
const (
	VariantProMax = "Pro Max"
	VariantPro    = "Pro"
	VariantPlus   = "Plus"
	VariantBase   = "無"
)

// titlePattern selects listings worth reading, from the listing page alone.
// iPhone listings are about a fifth of MacShop, so four in five articles cost
// nothing beyond the index page that was fetched anyway.
//
// The generation range runs to 19, so a model released later is picked up
// without a change here.
var titlePattern = regexp.MustCompile(`(?i)i?phone\s*1[2-9]`)

var (
	queryModel = regexp.MustCompile(`(?i)\b(1[2-9])\b`)
	queryMax   = regexp.MustCompile(`(?i)pro\s*max`)
	queryPro   = regexp.MustCompile(`(?i)\bpro\b`)
	queryPlus  = regexp.MustCompile(`(?i)\bplus\b`)
	// Capacity has to be recognised before the generation is read, or the 256 in
	// "17 Pro 256" would be taken for a model number.
	queryCapacity = regexp.MustCompile(`(?i)(\d+)\s*(tb|gb|g)\b|\b(128|256|512|1024)\b`)
)

// Kind is the iPhone kind.
type Kind struct{}

func init() { market.Register(Kind{}) }

func (Kind) Name() string { return "iphone" }

func (Kind) TitlePattern() *regexp.Regexp { return titlePattern }

// GroupBy orders the attributes a distribution is grouped by. Capacity comes
// last because it is the one a reader most often leaves out.
func (Kind) GroupBy() []string { return []string{AttrModel, AttrVariant, AttrCapacity} }

// AttrLabel names an attribute for a reader.
func (Kind) AttrLabel(name string) string {
	switch name {
	case AttrModel:
		return "機型"
	case AttrVariant:
		return "版本"
	case AttrCapacity:
		return "容量"
	case AttrBattery:
		return "電池健康度"
	default:
		return name
	}
}

// AttrValue renders a stored value the way it is written on the board.
func (Kind) AttrValue(name, value string) string {
	switch name {
	case AttrCapacity:
		return capacityLabel(value)
	case AttrBattery:
		return value + "%"
	default:
		return value
	}
}

func (Kind) AttrSchema() map[string]interface{} {
	return map[string]interface{}{
		AttrModel: map[string]interface{}{"type": "string"},
		AttrVariant: map[string]interface{}{
			"type": "string",
			"enum": []string{VariantProMax, VariantPro, VariantPlus, VariantBase},
		},
		AttrCapacity: map[string]interface{}{"type": "string"},
		AttrBattery:  map[string]interface{}{"type": "string"},
	}
}

func (Kind) PromptRules() string {
	return `7. 商品是 iPhone 手機本體時才填寫 attrs。
   保護殼、保護貼、充電器、轉接線、耳機等配件**不是手機本體**，
   即使名稱裡有「iPhone 16」也一律把 attrs 整個留空。
   - model：世代數字，例如 "17"、"16"。不是 iPhone 手機本體就留空。
   - variant：Pro Max / Pro / Plus / 無。注意「17 Pro Max」的 variant 是 Pro Max 不是 Pro。
   - capacity：容量的 GB 數字串。商品名稱或內文只要出現 256G / 512GB / 1TB 等字樣就必須填，
     1TB 填 "1024"、2TB 填 "2048"。真的完全沒提到才留空。
   - battery：電池健康度的百分比數字，沒寫就留空。全新未拆可填 "100"。
`
}

// Attrs converts an extracted item into recorded attributes.
//
// The extractor decides whether the item is a handset at all: it leaves attrs
// empty for an accessory, which is what keeps "iPhone 16 原廠矽膠保護殼" at 699
// out of an iPhone 16 distribution it would drag down by an order of magnitude.
//
// What it is not reliable about is copying details that are plainly in the
// item's own name -- capacity in particular is often left out -- so anything
// missing is read from the name, which the board's rules require to carry it.
// A capacity is then required: without one there is nothing to compare like
// with like.
func (k Kind) Attrs(item price.Item) (market.Attrs, bool) {
	if len(item.Attrs) == 0 {
		return nil, false
	}
	fromName := k.ParseQuery(item.Name)
	model := firstNonEmpty(item.Attrs[AttrModel], fromName.Get(AttrModel))
	capacity := normaliseCapacity(firstNonEmpty(item.Attrs[AttrCapacity], fromName.Get(AttrCapacity)))
	if model == "" || capacity == "" {
		return nil, false
	}
	attrs := market.Attrs{AttrModel: model, AttrCapacity: capacity}
	// The extractor reads "pm" and "ProMax" as Pro Max where a pattern would
	// not, so its answer is preferred over the name.
	if variant := firstNonEmpty(item.Attrs[AttrVariant], fromName.Get(AttrVariant)); variant != "" {
		attrs[AttrVariant] = variant
	}
	if battery := normaliseBattery(item.Attrs[AttrBattery]); battery != "" {
		attrs[AttrBattery] = battery
	}
	return attrs, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// normaliseCapacity accepts what the extractor actually returns -- "256",
// "256G", "512 GB", "1TB" -- and yields a bare count of gigabytes.
func normaliseCapacity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return ""
	}
	if gb, err := strconv.Atoi(raw); err == nil {
		if gb <= 0 {
			return ""
		}
		return strconv.Itoa(gb)
	}
	return parseCapacity(raw)
}

// normaliseBattery accepts "92" and "92%", and drops anything outside the range
// a battery can report.
func normaliseBattery(raw string) string {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "%")
	health, err := strconv.Atoi(raw)
	if err != nil || health <= 0 || health > 100 {
		return ""
	}
	return strconv.Itoa(health)
}

// ParseQuery reads attributes out of text such as "iPhone 17 Pro Max 256".
// A query with no generation is not an iPhone query at all, which is how the
// registry decides which kind a lookup belongs to.
func (Kind) ParseQuery(text string) market.Attrs {
	attrs := market.Attrs{}
	if model := queryModel.FindStringSubmatch(queryCapacity.ReplaceAllString(text, " ")); len(model) > 1 {
		attrs[AttrModel] = model[1]
	} else {
		return nil
	}
	switch {
	case queryMax.MatchString(text):
		attrs[AttrVariant] = VariantProMax
	case queryPro.MatchString(text):
		attrs[AttrVariant] = VariantPro
	case queryPlus.MatchString(text):
		attrs[AttrVariant] = VariantPlus
	}
	if capacity := parseCapacity(text); capacity != "" {
		attrs[AttrCapacity] = capacity
	}
	return attrs
}

func parseCapacity(text string) string {
	matches := queryCapacity.FindStringSubmatch(text)
	if matches == nil {
		return ""
	}
	if matches[3] != "" {
		return matches[3]
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return ""
	}
	if strings.EqualFold(matches[2], "tb") {
		value *= 1024
	}
	return strconv.Itoa(value)
}

// Label renders attributes as a product name.
func (Kind) Label(attrs market.Attrs) string {
	name := "iPhone " + attrs.Get(AttrModel)
	if variant := attrs.Get(AttrVariant); variant != "" && variant != VariantBase {
		name += " " + variant
	}
	if capacity := attrs.Get(AttrCapacity); capacity != "" {
		name += " " + capacityLabel(capacity)
	}
	return name
}

func capacityLabel(capacity string) string {
	gb, err := strconv.Atoi(capacity)
	if err != nil {
		return capacity
	}
	if gb >= 1024 && gb%1024 == 0 {
		return strconv.Itoa(gb/1024) + "TB"
	}
	return strconv.Itoa(gb) + "G"
}

// DedupeKey identifies one handset. Without a capacity the attributes are too
// coarse to tell one of a seller's phones from another -- two same-day listings
// by one seller are different phones, since the board forbids reposting within
// ten days -- so such records are left unmerged.
func (Kind) DedupeKey(attrs market.Attrs) (string, bool) {
	if !attrs.Has(AttrModel, AttrCapacity) {
		return "", false
	}
	return strings.Join([]string{
		attrs.Get(AttrModel), attrs.Get(AttrVariant), attrs.Get(AttrCapacity),
	}, "|"), true
}
