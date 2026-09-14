package subscription

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

var EmptyPushSum = PushSum{}

// Subscription is a struct show User Subscription
type Subscription struct {
	Board    string             `json:"board"`
	Keywords myutil.StringSlice `json:"keywords"`
	Authors  myutil.StringSlice `json:"authors"`
	Articles myutil.StringSlice `json:"articles"`
	PushSum  `json:"pushSum"`
	// MaxPrices holds the price ceiling for keywords that have one, keyed by the
	// lowercased keyword. A keyword absent here is matched on its title alone,
	// which is the behaviour every subscription had before price tracking, so
	// subscriptions stored without this field keep working untouched.
	MaxPrices map[string]int `json:"maxPrices,omitempty"`
}

type PushSum struct {
	Up   int `json:"up"`
	Down int `json:"down"`
}

func (s Subscription) String() string {
	if len(s.Keywords) == 0 {
		return ""
	}
	sort.Strings(s.Keywords)
	return s.Board + ": " + strings.Join(s.Keywords, ", ")
}

func (s Subscription) StringAuthor() string {
	if len(s.Authors) == 0 {
		return ""
	}
	sort.Strings(s.Authors)
	return s.Board + ": " + strings.Join(s.Authors, ", ")
}

func (s Subscription) StringPushSum() string {
	emptyPushSum := PushSum{}
	if s.PushSum == emptyPushSum {
		return ""
	}
	return fmt.Sprintf("%s: 推 %d; 噓 %d", s.Board, s.PushSum.Up, s.PushSum.Down)
}

func (s Subscription) StringArticle() string {
	if len(s.Articles) == 0 {
		return ""
	}
	sort.Strings(s.Articles)
	aURLs := make([]string, 0)
	for _, a := range s.Articles {
		aURLs = append(aURLs, buildArticleURL(s.Board, a))
	}
	return s.Board + ":\n" + strings.Join(aURLs, "\n")
}

func buildArticleURL(board, code string) string {
	return fmt.Sprintf("https://www.ptt.cc/bbs/%s/%s.html", board, code)
}

func (s *Subscription) CleanUp() {
	s.Keywords.Clean()
	s.Authors.Clean()
	s.Authors.RemoveStringsSpace()
}

func (s *Subscription) DeleteKeywords(keywords myutil.StringSlice) {
	s.Keywords.Delete(keywords, false)
	for _, keyword := range keywords {
		delete(s.MaxPrices, strings.ToLower(keyword))
	}
	if len(s.MaxPrices) == 0 {
		s.MaxPrices = nil
	}
}

// SetMaxPrice attaches a price ceiling to a keyword. A ceiling of zero or less
// removes it, mirroring how a push sum of zero cancels a push subscription.
func (s *Subscription) SetMaxPrice(keyword string, maxPrice int) {
	key := strings.ToLower(keyword)
	if maxPrice <= 0 {
		delete(s.MaxPrices, key)
		if len(s.MaxPrices) == 0 {
			s.MaxPrices = nil
		}
		return
	}
	if s.MaxPrices == nil {
		s.MaxPrices = make(map[string]int)
	}
	s.MaxPrices[key] = maxPrice
}

// MaxPrice reports the ceiling set for a keyword, if any.
func (s Subscription) MaxPrice(keyword string) (int, bool) {
	maxPrice, ok := s.MaxPrices[strings.ToLower(keyword)]
	return maxPrice, ok
}

// StringMaxPrice renders the board's price ceilings for the subscription list.
func (s Subscription) StringMaxPrice() string {
	if len(s.MaxPrices) == 0 {
		return ""
	}
	keywords := make([]string, 0, len(s.MaxPrices))
	for keyword := range s.MaxPrices {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	lines := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		lines = append(lines, fmt.Sprintf("%s: %s 上限 %d", s.Board, keyword, s.MaxPrices[keyword]))
	}
	return strings.Join(lines, "\n")
}

func (s *Subscription) DeleteAuthors(authors myutil.StringSlice) {
	s.Authors.Delete(authors, false)
}

func (s *Subscription) DeleteArticles(articles myutil.StringSlice) {
	s.Articles.Delete(articles, false)
}
