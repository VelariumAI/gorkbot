package research

import "time"

// SearchResult represents a single web search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Rank    int    `json:"rank"`
}

// Document represents a fetched web page stored in the buffer.
type Document struct {
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Content     string    `json:"-"` // never serialized
	Length      int       `json:"length"`
	FetchedAt   time.Time `json:"fetched_at"`
	ContentHash string    `json:"content_hash"`
}

// FindMatch represents a pattern match within a document.
type FindMatch struct {
	LineNumber  int    `json:"line_number"`
	MatchText   string `json:"match_text"`
	Context     string `json:"context"`
	StartOffset int    `json:"start_offset"`
}

// DocumentSummary is a lightweight view of a document without content.
type DocumentSummary struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Length   int    `json:"length"`
	IsActive bool   `json:"is_active"`
}
