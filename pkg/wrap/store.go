package wrap

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxStoredLinks  = 25000
	maxReferrerKeys = 16
)

var (
	ErrLinkNotFound = errors.New("short link not found")
	ErrLinkExpired  = errors.New("short link has expired")
	ErrAliasTaken   = errors.New("custom alias is already in use")
)

// LinkRecord holds the target URL, lifecycle timestamps, and aggregated click telemetry.
type LinkRecord struct {
	Code        string         `json:"code"`
	URL         string         `json:"url"`
	Stateless   string         `json:"stateless,omitempty"`
	CreatedAt   int64          `json:"createdAt"`
	ExpiresAt   int64          `json:"expiresAt,omitempty"`
	Clicks      int64          `json:"clicks"`
	LastClicked int64          `json:"lastClicked,omitempty"`
	Referrers   map[string]int `json:"referrers,omitempty"`
}

// IsExpired reports whether the link has passed its expiration timestamp.
func (lr *LinkRecord) IsExpired(now int64) bool {
	return lr.ExpiresAt > 0 && now > lr.ExpiresAt
}

// Store manages short links in memory with optional JSON file persistence.
type Store struct {
	mu       sync.RWMutex
	links    map[string]*LinkRecord
	filePath string
}

// NewStore initializes a Store and loads existing links from disk if a snapshot exists.
func NewStore(snapshotPath string) *Store {
	s := &Store{
		links:    make(map[string]*LinkRecord),
		filePath: snapshotPath,
	}
	if snapshotPath != "" {
		s.loadFromDisk()
	}
	return s
}

// Put saves a new short link record, enforcing alias uniqueness and memory bounds.
func (s *Store) Put(record *LinkRecord, customAlias bool) error {
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.links[record.Code]; exists {
		if !existing.IsExpired(now) {
			if customAlias {
				return ErrAliasTaken
			}
			return ErrAliasTaken
		}
		delete(s.links, record.Code)
	}

	if len(s.links) >= maxStoredLinks {
		s.evict(now)
	}

	if record.Referrers == nil {
		record.Referrers = make(map[string]int)
	}
	s.links[record.Code] = record
	s.saveToDiskLocked()
	return nil
}

// Resolve looks up a short code, checks expiration, and records a visit click.
func (s *Store) Resolve(code string, rawReferrer string) (*LinkRecord, error) {
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.links[code]
	if !exists {
		return nil, ErrLinkNotFound
	}

	if rec.IsExpired(now) {
		delete(s.links, code)
		s.saveToDiskLocked()
		return nil, ErrLinkExpired
	}

	rec.Clicks++
	rec.LastClicked = now

	refHost := normalizeReferrer(rawReferrer)
	if refHost != "" {
		if rec.Referrers == nil {
			rec.Referrers = make(map[string]int)
		}
		if _, ok := rec.Referrers[refHost]; ok || len(rec.Referrers) < maxReferrerKeys {
			rec.Referrers[refHost]++
		}
	}

	s.saveToDiskLocked()
	return cloneRecord(rec), nil
}

// Get retrieves link statistics without incrementing the click counter.
func (s *Store) Get(code string) (*LinkRecord, error) {
	now := time.Now().Unix()

	s.mu.RLock()
	rec, exists := s.links[code]
	if !exists {
		s.mu.RUnlock()
		return nil, ErrLinkNotFound
	}
	if rec.IsExpired(now) {
		s.mu.RUnlock()
		return nil, ErrLinkExpired
	}
	out := cloneRecord(rec)
	s.mu.RUnlock()
	return out, nil
}

// Count returns the number of active links currently held in memory.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.links)
}

func (s *Store) evict(now int64) {
	var oldestCode string
	var oldestCreated int64

	for k, rec := range s.links {
		if rec.IsExpired(now) {
			delete(s.links, k)
			continue
		}
		if oldestCode == "" || rec.CreatedAt < oldestCreated {
			oldestCode = k
			oldestCreated = rec.CreatedAt
		}
	}

	if len(s.links) >= maxStoredLinks && oldestCode != "" {
		delete(s.links, oldestCode)
	}
}

func (s *Store) loadFromDisk() {
	data, err := os.ReadFile(s.filePath)
	if err != nil || len(data) == 0 {
		return
	}
	var items []*LinkRecord
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}
	now := time.Now().Unix()
	for _, item := range items {
		if item != nil && item.Code != "" && !item.IsExpired(now) {
			if item.Referrers == nil {
				item.Referrers = make(map[string]int)
			}
			s.links[item.Code] = item
		}
	}
}

func (s *Store) saveToDiskLocked() {
	if s.filePath == "" {
		return
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}

	items := make([]*LinkRecord, 0, len(s.links))
	for _, rec := range s.links {
		items = append(items, rec)
	}

	data, err := json.Marshal(items)
	if err != nil {
		return
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmpPath, s.filePath)
}

func cloneRecord(rec *LinkRecord) *LinkRecord {
	refs := make(map[string]int, len(rec.Referrers))
	for k, v := range rec.Referrers {
		refs[k] = v
	}
	return &LinkRecord{
		Code:        rec.Code,
		URL:         rec.URL,
		Stateless:   rec.Stateless,
		CreatedAt:   rec.CreatedAt,
		ExpiresAt:   rec.ExpiresAt,
		Clicks:      rec.Clicks,
		LastClicked: rec.LastClicked,
		Referrers:   refs,
	}
}

func normalizeReferrer(raw string) string {
	if raw == "" {
		return "direct"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "direct"
	}
	return u.Hostname()
}
