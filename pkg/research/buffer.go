package research

import "sync"

// DocBuffer is a ring-buffer document store. Document content stays here
// and never enters conversation history.
type DocBuffer struct {
	docs    map[string]*Document
	order   []string
	maxDocs int
	active  string
	mu      sync.RWMutex
}

// NewDocBuffer creates a buffer that holds at most maxDocs documents.
func NewDocBuffer(maxDocs int) *DocBuffer {
	if maxDocs <= 0 {
		maxDocs = 10
	}
	return &DocBuffer{
		docs:    make(map[string]*Document),
		maxDocs: maxDocs,
	}
}

// Store adds a document to the buffer, evicting the oldest if full.
func (b *DocBuffer) Store(doc *Document) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If already stored, update in place
	if _, exists := b.docs[doc.URL]; exists {
		b.docs[doc.URL] = doc
		return
	}

	// Evict oldest if at capacity
	for len(b.order) >= b.maxDocs {
		oldest := b.order[0]
		b.order = b.order[1:]
		delete(b.docs, oldest)
		if b.active == oldest {
			b.active = ""
		}
	}

	b.docs[doc.URL] = doc
	b.order = append(b.order, doc.URL)
}

// Get retrieves a document by URL.
func (b *DocBuffer) Get(url string) (*Document, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	doc, ok := b.docs[url]
	return doc, ok
}

// SetActive sets the active document by URL. Returns false if not found.
func (b *DocBuffer) SetActive(url string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.docs[url]; !ok {
		return false
	}
	b.active = url
	return true
}

// Active returns the currently active document, or nil.
func (b *DocBuffer) Active() *Document {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.active == "" {
		return nil
	}
	return b.docs[b.active]
}

// List returns lightweight summaries of all buffered documents.
func (b *DocBuffer) List() []DocumentSummary {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]DocumentSummary, 0, len(b.order))
	for _, url := range b.order {
		doc := b.docs[url]
		out = append(out, DocumentSummary{
			URL:      doc.URL,
			Title:    doc.Title,
			Length:   doc.Length,
			IsActive: url == b.active,
		})
	}
	return out
}

// Clear removes all documents from the buffer.
func (b *DocBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.docs = make(map[string]*Document)
	b.order = nil
	b.active = ""
}

// Count returns the number of buffered documents.
func (b *DocBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.docs)
}
