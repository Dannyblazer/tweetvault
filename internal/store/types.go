package store

import "time"

// ContentType enumerates what kind of thing a ContentItem wraps.
type ContentType string

const (
	TypeTweet ContentType = "tweet"
	TypeVideo ContentType = "video"
	TypeLink  ContentType = "link"
	TypeNote  ContentType = "note"
)

// Status is the pipeline stage of a ContentItem.
type Status string

const (
	StatusInbox    Status = "inbox"
	StatusTagged   Status = "tagged"
	StatusUsed     Status = "used"
	StatusArchived Status = "archived"
)

// ContentItem is the atomic saved unit: a tweet, video, link, or note.
type ContentItem struct {
	ID           string      `json:"id"`
	Type         ContentType `json:"type"`
	SourceURL    string      `json:"source_url,omitempty"`
	CapturedAt   time.Time   `json:"captured_at"`
	Title        string      `json:"title,omitempty"`
	ThumbnailURL string      `json:"thumbnail_url,omitempty"`
	TextContent  string      `json:"text_content,omitempty"` // tweet text / transcript / article body — searched
	Notes        string      `json:"notes,omitempty"`        // your annotation: why it's funny/relatable
	TagIDs       []string    `json:"tag_ids"`
	Status       Status      `json:"status"`
}

// Tag is a canonical label. Variant spellings resolve to it via Aliases.
type Tag struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"` // canonical, normalized (lowercase, trimmed)
	Aliases []string `json:"aliases,omitempty"`
	Facet   string   `json:"facet,omitempty"` // optional: theme / emotion / format / audience / trend
}

// DraftStatus is whether a draft has been posted yet.
type DraftStatus string

const (
	DraftStatusDraft     DraftStatus = "draft"
	DraftStatusPublished DraftStatus = "published"
)

// Draft is a tweet-in-progress, linked back to the ContentItems that inspired it.
type Draft struct {
	ID                   string      `json:"id"`
	Text                 string      `json:"text"`
	LinkedContentItemIDs []string    `json:"linked_content_item_ids"`
	InheritedTagIDs      []string    `json:"inherited_tag_ids"`
	Status               DraftStatus `json:"status"`
	PublishedURL         string      `json:"published_url,omitempty"`
	CreatedAt            time.Time   `json:"created_at"`
	PublishedAt          *time.Time  `json:"published_at,omitempty"`
}
