// Package market keeps a history of asking prices seen on a second-hand board so
// that a single listing can be read against what comparable items are going for.
//
// Records are appended to one JSON Lines file per month under storage/market.
// At the volume these boards produce -- a few thousand rows a year -- loading a
// window into memory and aggregating there is immediate, and the files stay
// readable with ordinary tools, which a database file would not.
package market

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Record is one asking price observed in one article.
type Record struct {
	// Kind names the class of goods, so records of different classes can share
	// one file and one query path.
	Kind   string `json:"kind"`
	Board  string `json:"board"`
	Code   string `json:"code"`
	Author string `json:"author"`
	// PostedAt is when the article appeared; ObservedAt is when its price was
	// read. They differ, and both matter: sellers edit prices after posting, so
	// a record is only the price as it stood when it was read.
	PostedAt   time.Time `json:"postedAt"`
	ObservedAt time.Time `json:"observedAt"`
	// Attrs identify the product, in whatever terms its Kind defines.
	Attrs Attrs `json:"attrs"`
	Price int   `json:"price"`
	// Sold marks an article the seller has marked as sold. Unlike the alerting
	// path, which skips them, they are kept here: a price that found a buyer is
	// the most informative observation on the board.
	Sold     bool   `json:"sold"`
	PostType string `json:"postType"`
	Title    string `json:"title"`
}

var storeMu sync.Mutex

func storeDir() string { return filepath.Join(myutil.StoragePath(), "market") }

func monthFile(t time.Time) string {
	return filepath.Join(storeDir(), t.Format("2006-01")+".jsonl")
}

// Append writes records to the month files their articles belong to.
func Append(records []Record) error {
	if len(records) == 0 {
		return nil
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := os.MkdirAll(storeDir(), 0755); err != nil {
		return err
	}
	byMonth := make(map[string][]Record)
	for _, record := range records {
		byMonth[monthFile(record.PostedAt)] = append(byMonth[monthFile(record.PostedAt)], record)
	}
	for path, monthRecords := range byMonth {
		if err := appendFile(path, monthRecords); err != nil {
			return err
		}
	}
	return nil
}

func appendFile(path string, records []Record) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// Load returns every record posted at or after since, newest last.
func Load(since time.Time) ([]Record, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	entries, err := os.ReadDir(storeDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	records := make([]Record, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		// Month files are named for the month they hold, so a file entirely
		// older than the window can be skipped without reading it.
		if month, err := time.Parse("2006-01", strings.TrimSuffix(entry.Name(), ".jsonl")); err == nil {
			if month.AddDate(0, 1, 0).Before(since) {
				continue
			}
		}
		loaded, err := loadFile(filepath.Join(storeDir(), entry.Name()), since)
		if err != nil {
			return nil, err
		}
		records = append(records, loaded...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].PostedAt.Before(records[j].PostedAt) })
	return records, nil
}

func loadFile(path string, since time.Time) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records := make([]Record, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			log.WithField("file", path).WithError(err).Warn("Market Record Decode Failed")
			continue
		}
		if record.PostedAt.Before(since) {
			continue
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

// History returns the article codes already recorded, so a survey does not pay
// to extract an article it has already read, along with the most recent posting
// among them. That watermark is how a survey knows when it has caught up: it
// keeps walking back until it reaches articles it already holds, so a gap left
// by downtime is filled rather than stepped over.
//
// The watermark is zero when nothing is recorded yet.
func History(since time.Time) (map[string]bool, time.Time, error) {
	records, err := Load(since)
	if err != nil {
		return nil, time.Time{}, err
	}
	seen := make(map[string]bool, len(records))
	watermark := time.Time{}
	for _, record := range records {
		seen[record.Code] = true
		if record.PostedAt.After(watermark) {
			watermark = record.PostedAt
		}
	}
	return seen, watermark, nil
}
