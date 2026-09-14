package market

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	specModel = regexp.MustCompile(`(?i)\b(1[2-9])\b`)
	// Pro Max has to be tested before Pro: "17 Pro Max" contains "Pro", so a
	// check for Pro first would classify every Max as a plain Pro.
	specProMax   = regexp.MustCompile(`(?i)pro\s*max`)
	specPro      = regexp.MustCompile(`(?i)\bpro\b`)
	specPlus     = regexp.MustCompile(`(?i)\bplus\b`)
	specCapacity = regexp.MustCompile(`(?i)(\d+)\s*(tb|gb|g)\b|\b(128|256|512)\b`)
)

// ParseSpec reads a spec out of what someone typed, such as
// "iPhone 17 Pro Max 256". Anything it cannot find is left zero, which matches
// every value of that field rather than none.
func ParseSpec(text string) Spec {
	spec := Spec{}
	if model := specModel.FindStringSubmatch(stripCapacity(text)); len(model) > 1 {
		spec.Model = model[1]
	}
	switch {
	case specProMax.MatchString(text):
		spec.Variant = VariantProMax
	case specPro.MatchString(text):
		spec.Variant = VariantPro
	case specPlus.MatchString(text):
		spec.Variant = VariantPlus
	}
	spec.CapacityGB = parseCapacity(text)
	return spec
}

// stripCapacity removes capacities before the model is read, so "256" in
// "17 Pro 256" cannot be mistaken for a model number.
func stripCapacity(text string) string {
	return specCapacity.ReplaceAllString(text, " ")
}

func parseCapacity(text string) int {
	matches := specCapacity.FindStringSubmatch(text)
	if matches == nil {
		return 0
	}
	if matches[3] != "" {
		capacity, _ := strconv.Atoi(matches[3])
		return capacity
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	if strings.EqualFold(matches[2], "tb") {
		return value * 1024
	}
	return value
}

// VariantProMax and friends live in the price package; mirror them here so the
// market package does not drag price into every caller.
const (
	VariantProMax = "Pro Max"
	VariantPro    = "Pro"
	VariantPlus   = "Plus"
)
