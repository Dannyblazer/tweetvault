// Package store holds the vault's data: content items, tags, and drafts.
// It's an in-memory store backed by a single JSON file on disk — enough
// durability for a personal tool without pulling in a database dependency.
package store

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Store is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	path string // where the JSON snapshot is written; empty means in-memory only

	ContentItems map[string]*ContentItem `json:"content_items"`
	Tags         map[string]*Tag         `json:"tags"`
	Drafts       map[string]*Draft       `json:"drafts"`
}

// New creates an empty store. If path is non-empty, Save persists to it and
// Load(path) can restore it later.
func New(path string) *Store {
	return &Store{
		path:         path,
		ContentItems: map[string]*ContentItem{},
		Tags:         map[string]*Tag{},
		Drafts:       map[string]*Draft{},
	}
}

// Load reads a JSON snapshot from disk. If the file doesn't exist yet, it
// returns a fresh empty store rather than an error — first run of the app.
func Load(path string) (*Store, error) {
	s := New(path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading store file: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing store file: %w", err)
	}
	return s, nil
}

// Save writes the current state to disk as JSON. Called after every mutation
// so the vault survives a restart. No-op if the store has no path (tests).
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshaling store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing store file: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// NewID returns a short random hex ID. Good enough for a single-user tool;
// no coordination needed across machines.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// --- ContentItem operations ---

func (s *Store) CreateContentItem(item *ContentItem) *ContentItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	item.ID = NewID()
	if item.CapturedAt.IsZero() {
		item.CapturedAt = time.Now().UTC()
	}
	if item.Status == "" {
		item.Status = StatusInbox
	}
	s.ContentItems[item.ID] = item
	return item
}

func (s *Store) GetContentItem(id string) (*ContentItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.ContentItems[id]
	return item, ok
}

// ListContentItems returns all items, optionally filtered by status.
// Pass "" for status to get everything. Results are not sorted; callers
// (e.g. the search package) apply their own ordering.
func (s *Store) ListContentItems(status Status) []*ContentItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ContentItem, 0, len(s.ContentItems))
	for _, item := range s.ContentItems {
		if status == "" || item.Status == status {
			out = append(out, item)
		}
	}
	return out
}

func (s *Store) SetContentItemStatus(id string, status Status) (*ContentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.ContentItems[id]
	if !ok {
		return nil, fmt.Errorf("content item %q not found", id)
	}
	item.Status = status
	return item, nil
}

// AddTagsToContentItem attaches tag IDs to an item (deduped) and, if the
// item is still sitting untouched in the inbox, advances it to "tagged".
func (s *Store) AddTagsToContentItem(itemID string, tagIDs []string) (*ContentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.ContentItems[itemID]
	if !ok {
		return nil, fmt.Errorf("content item %q not found", itemID)
	}
	existing := map[string]bool{}
	for _, id := range item.TagIDs {
		existing[id] = true
	}
	for _, id := range tagIDs {
		if !existing[id] {
			item.TagIDs = append(item.TagIDs, id)
			existing[id] = true
		}
	}
	if item.Status == StatusInbox {
		item.Status = StatusTagged
	}
	return item, nil
}

// --- Tag operations ---

// FindTagByName looks for a tag by its canonical name OR any of its aliases.
// Name matching is case-insensitive; callers should normalize first (see
// the tagging package) but this is defensive either way.
func (s *Store) FindTagByName(name string) (*Tag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.Tags {
		if t.Name == name {
			return t, true
		}
		for _, a := range t.Aliases {
			if a == name {
				return t, true
			}
		}
	}
	return nil, false
}

func (s *Store) CreateTag(name string, facet string) *Tag {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Tag{ID: NewID(), Name: name, Facet: facet}
	s.Tags[t.ID] = t
	return t
}

func (s *Store) GetTag(id string) (*Tag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.Tags[id]
	return t, ok
}

func (s *Store) ListTags() []*Tag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Tag, 0, len(s.Tags))
	for _, t := range s.Tags {
		out = append(out, t)
	}
	return out
}

// TagUsageCount counts how many content items currently carry this tag.
func (s *Store) TagUsageCount(tagID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, item := range s.ContentItems {
		for _, id := range item.TagIDs {
			if id == tagID {
				n++
				break
			}
		}
	}
	return n
}

// MergeTags folds "from" into "to": every content item tagged "from" gets
// retagged "to", "from"'s name becomes an alias of "to", and the "from" tag
// record is removed. This is how tag vocabulary drift gets cleaned up
// without losing history on old items.
func (s *Store) MergeTags(fromID, toID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, ok := s.Tags[fromID]
	if !ok {
		return fmt.Errorf("tag %q not found", fromID)
	}
	to, ok := s.Tags[toID]
	if !ok {
		return fmt.Errorf("tag %q not found", toID)
	}
	if fromID == toID {
		return fmt.Errorf("cannot merge a tag into itself")
	}
	for _, item := range s.ContentItems {
		hasFrom, hasTo := false, false
		kept := item.TagIDs[:0]
		for _, id := range item.TagIDs {
			switch id {
			case fromID:
				hasFrom = true
			case toID:
				hasTo = true
				kept = append(kept, id)
			default:
				kept = append(kept, id)
			}
		}
		if hasFrom && !hasTo {
			kept = append(kept, toID)
		}
		item.TagIDs = kept
	}
	to.Aliases = append(to.Aliases, from.Name)
	to.Aliases = append(to.Aliases, from.Aliases...)
	delete(s.Tags, fromID)
	return nil
}

// --- Draft operations ---

func (s *Store) CreateDraft(d *Draft) *Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	d.ID = NewID()
	d.CreatedAt = time.Now().UTC()
	if d.Status == "" {
		d.Status = DraftStatusDraft
	}
	s.Drafts[d.ID] = d
	return d
}

func (s *Store) GetDraft(id string) (*Draft, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.Drafts[id]
	return d, ok
}

func (s *Store) ListDrafts() []*Draft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Draft, 0, len(s.Drafts))
	for _, d := range s.Drafts {
		out = append(out, d)
	}
	return out
}

func (s *Store) UpdateDraftText(id, text string) (*Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Drafts[id]
	if !ok {
		return nil, fmt.Errorf("draft %q not found", id)
	}
	d.Text = text
	return d, nil
}

// PublishDraft marks a draft published and advances every ContentItem it
// was built from to "used" — closing the loop described in the design doc.
func (s *Store) PublishDraft(id, publishedURL string) (*Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Drafts[id]
	if !ok {
		return nil, fmt.Errorf("draft %q not found", id)
	}
	d.Status = DraftStatusPublished
	d.PublishedURL = publishedURL
	now := time.Now().UTC()
	d.PublishedAt = &now
	for _, itemID := range d.LinkedContentItemIDs {
		if item, ok := s.ContentItems[itemID]; ok {
			item.Status = StatusUsed
		}
	}
	return d, nil
}
