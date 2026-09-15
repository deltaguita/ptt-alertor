package command

import "testing"

func TestAddMaxPriceSyntax(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantBoard    string
		wantKeyword  string
		wantMaxPrice string
		wantNoMatch  bool
	}{
		{
			name:      "keyword containing spaces",
			input:     "新增售價 macshop iPhone 17 Pro Max 35000",
			wantBoard: "macshop", wantKeyword: "iPhone 17 Pro Max", wantMaxPrice: "35000",
		},
		{
			name:      "single word keyword",
			input:     "新增售價 macshop AirPods 4000",
			wantBoard: "macshop", wantKeyword: "AirPods", wantMaxPrice: "4000",
		},
		{
			name:      "several boards",
			input:     "新增售價 macshop,nb-shopping MacBook Air 30000",
			wantBoard: "macshop,nb-shopping", wantKeyword: "MacBook Air", wantMaxPrice: "30000",
		},
		{
			name:      "zero removes the ceiling",
			input:     "新增售價 macshop AirPods 0",
			wantBoard: "macshop", wantKeyword: "AirPods", wantMaxPrice: "0",
		},
		{
			name:      "a keyword ending in digits still parses",
			input:     "新增售價 macshop iPhone 17 40000",
			wantBoard: "macshop", wantKeyword: "iPhone 17", wantMaxPrice: "40000",
		},
		{name: "no ceiling at all", input: "新增售價 macshop iPhone", wantNoMatch: true},
		{name: "no keyword", input: "新增售價 macshop", wantNoMatch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := addMaxPriceRe.FindStringSubmatch(tt.input)
			if tt.wantNoMatch {
				if args != nil {
					t.Errorf("%q parsed as %v, want a usage error", tt.input, args[1:])
				}
				return
			}
			if args == nil {
				t.Fatalf("%q did not parse", tt.input)
			}
			if args[1] != tt.wantBoard || args[2] != tt.wantKeyword || args[3] != tt.wantMaxPrice {
				t.Errorf("%q -> board=%q keyword=%q max=%q, want %q/%q/%q",
					tt.input, args[1], args[2], args[3],
					tt.wantBoard, tt.wantKeyword, tt.wantMaxPrice)
			}
		})
	}
}

func TestRemoveMaxPriceSyntax(t *testing.T) {
	args := removeMaxPriceRe.FindStringSubmatch("刪除售價 macshop iPhone 17 Pro Max")
	if args == nil {
		t.Fatal("did not parse")
	}
	if args[1] != "macshop" || args[2] != "iPhone 17 Pro Max" {
		t.Errorf("board=%q keyword=%q", args[1], args[2])
	}
}
