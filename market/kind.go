package market

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Ptt-Alertor/ptt-alertor/price"
)

// Attrs are the identifying attributes of one product, keyed by names its Kind
// defines. They are strings because nothing below a Kind needs to interpret
// them: the survey stores them, the statistics group by them, and only the Kind
// that named them knows that "capacity" is measured in gigabytes.
type Attrs map[string]string

// Get returns an attribute, or "" when it is absent.
func (a Attrs) Get(name string) string { return a[name] }

// Has reports whether every named attribute is present and non-empty.
func (a Attrs) Has(names ...string) bool {
	for _, name := range names {
		if a[name] == "" {
			return false
		}
	}
	return true
}

// Matches reports whether a record's attributes satisfy a query. An attribute
// absent from the query matches any value, so "17 Pro" covers every capacity.
func (a Attrs) Matches(query Attrs) bool {
	for name, want := range query {
		if want == "" {
			continue
		}
		if !strings.EqualFold(a[name], want) {
			return false
		}
	}
	return true
}

// Kind is one class of goods the survey can track. Everything a class needs to
// differ in lives here; the storage, the scanning, the budgeting and the
// statistics are shared and know nothing about any particular product.
type Kind interface {
	// Name, AttrSchema and PromptRules are what the extractor needs.
	price.Kind

	// TitlePattern selects listings worth reading, from the listing page alone.
	// This is what keeps a survey affordable, so it should be as narrow as the
	// class allows.
	TitlePattern() *regexp.Regexp

	// Attrs converts an extracted item into the attributes this kind records.
	// It reports false for an item that is not of this kind at all -- a phone
	// case named "iPhone 16 保護殼" is not an iPhone -- which is what keeps
	// accessories out of a distribution they would distort.
	Attrs(item price.Item) (Attrs, bool)

	// ParseQuery reads attributes out of what someone typed, such as
	// "iPhone 17 Pro Max 256".
	ParseQuery(text string) Attrs

	// Label renders attributes as a product name for display.
	Label(attrs Attrs) string

	// DedupeKey identifies one unit for relist detection. It reports false when
	// the attributes are too coarse to tell one of a seller's items from
	// another, in which case the records are left alone rather than merged.
	DedupeKey(attrs Attrs) (string, bool)

	// GroupBy names the attributes a distribution is grouped by, most
	// significant first. A query that leaves a later one unset is told which
	// values it is averaging over.
	GroupBy() []string

	// AttrLabel names an attribute for a reader, and AttrValue renders one of
	// its values. Attribute keys and stored values are internal -- "capacity"
	// and "512" -- and showing them raw leaks the schema into the message.
	AttrLabel(name string) string
	AttrValue(name, value string) string
}

var (
	kindsMu sync.RWMutex
	kinds   = make(map[string]Kind)
)

// Register makes a kind available. A kind registers itself from an init
// function, so that adding a class of goods is a matter of writing its package
// and importing it -- this package never learns any product's name.
func Register(kind Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	kinds[kind.Name()] = kind
}

// KindByName returns a registered kind.
func KindByName(name string) (Kind, bool) {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	kind, ok := kinds[name]
	return kind, ok
}

// Kinds returns every registered kind, in a stable order.
func Kinds() []Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	registered := make([]Kind, 0, len(names))
	for _, name := range names {
		registered = append(registered, kinds[name])
	}
	return registered
}

// KindFor returns the kind whose query parser recognises what someone typed,
// so a lookup does not have to name its class.
func KindFor(text string) (Kind, Attrs, bool) {
	for _, kind := range Kinds() {
		if attrs := kind.ParseQuery(text); len(attrs) > 0 {
			return kind, attrs, true
		}
	}
	return nil, nil, false
}

// GroupingOf keeps only the attributes a distribution groups by, which is what a
// comparison must be keyed on. Recorded attributes include descriptive ones --
// a handset's battery health, say -- and querying on those would compare a phone
// only against others in identical condition, which is almost never anything.
func GroupingOf(kind Kind, attrs Attrs) Attrs {
	query := Attrs{}
	for _, name := range kind.GroupBy() {
		if value := attrs.Get(name); value != "" {
			query[name] = value
		}
	}
	return query
}

// KindForTitle returns the kind that claims a listing title.
func KindForTitle(title string) (Kind, bool) {
	for _, kind := range Kinds() {
		if kind.TitlePattern().MatchString(title) {
			return kind, true
		}
	}
	return nil, false
}

// Plain is the kind used when nothing else claims an article: it asks the
// extractor for a price and nothing more. It is deliberately not registered --
// it matches no title, so no survey reads with it -- and exists so the alerting
// path can price an article of any description.
var Plain Kind = plain{}

type plain struct{}

func (plain) Name() string                       { return "plain" }
func (plain) AttrSchema() map[string]interface{} { return map[string]interface{}{} }
func (plain) PromptRules() string                { return "" }
func (plain) TitlePattern() *regexp.Regexp       { return regexp.MustCompile(`$^`) }
func (plain) Attrs(price.Item) (Attrs, bool)     { return nil, false }
func (plain) ParseQuery(string) Attrs            { return nil }
func (plain) Label(Attrs) string                 { return "" }
func (plain) DedupeKey(Attrs) (string, bool)     { return "", false }
func (plain) GroupBy() []string                  { return nil }
func (plain) AttrLabel(name string) string       { return name }
func (plain) AttrValue(_, value string) string   { return value }
