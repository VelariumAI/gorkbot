// Package a2a — journal.go
//
// PendingJournal provides crash-safe persistence for outbound A2A messages
// using the same atomic-write pattern from build-your-own-openclaw Step 09:
//
//	Write path:  content → <dir>/<id>.tmp  → fsync → rename to <dir>/<id>.json
//	Read path:   scan <dir> for *.json files on startup and re-dispatch them.
//	Delete path: remove <dir>/<id>.json after confirmed delivery.
//
// The atomic rename guarantees that every file in the pending directory is
// either fully written or absent — no partial writes survive a crash.
//
// Usage (in Channel.SendQuery or wherever a message is dispatched):
//
//	j := a2a.NewPendingJournal(filepath.Join(configDir, "a2a-pending"))
//	if err := j.WritePending(msg); err != nil { ... }
//	// ... deliver msg ...
//	j.DeletePending(msg.ID) // idempotent
//
// Startup recovery:
//
//	msgs, _ := j.RecoverPending()
//	for _, m := range msgs { redispatch(m) }
package a2a

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PendingJournal durably stores outbound A2A messages until delivery is
// confirmed. It is safe for concurrent use.
type PendingJournal struct {
	dir string
	mu  sync.Mutex
}

// NewPendingJournal creates a PendingJournal rooted at dir.
// The directory is created if it does not exist.
func NewPendingJournal(dir string) (*PendingJournal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("a2a/journal: mkdir %s: %w", dir, err)
	}
	return &PendingJournal{dir: dir}, nil
}

// WritePending atomically persists msg to the journal.
// The write sequence is: encode → write .tmp → fsync → rename to .json.
// A crash at any point leaves the journal consistent.
func (j *PendingJournal) WritePending(msg Message) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("a2a/journal: marshal: %w", err)
	}

	// Write to a temporary file first.
	tmpPath := filepath.Join(j.dir, msg.ID+".tmp")
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("a2a/journal: open tmp: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("a2a/journal: write: %w", err)
	}

	// fsync before rename — guarantees data is on disk before the inode move.
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("a2a/journal: fsync: %w", err)
	}
	f.Close()

	// Atomic rename: this is the commit point.
	dest := filepath.Join(j.dir, msg.ID+".json")
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("a2a/journal: rename: %w", err)
	}
	return nil
}

// DeletePending removes the persisted record for msgID after delivery.
// Idempotent — returns nil if the file does not exist.
func (j *PendingJournal) DeletePending(msgID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	path := filepath.Join(j.dir, msgID+".json")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("a2a/journal: delete %s: %w", msgID, err)
	}
	return nil
}

// RecoverPending returns all messages that were written but not yet deleted.
// Call this on startup to re-dispatch messages that survived a crash.
// It is the caller's responsibility to call DeletePending after successful
// re-delivery of each returned message.
func (j *PendingJournal) RecoverPending() ([]Message, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("a2a/journal: readdir: %w", err)
	}

	var msgs []Message
	var errs []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(j.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read %s: %v", e.Name(), err))
			continue
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			// Corrupt file — remove it to avoid looping on bad data.
			errs = append(errs, fmt.Sprintf("corrupt %s: %v (removed)", e.Name(), err))
			os.Remove(path)
			continue
		}
		msgs = append(msgs, msg)
	}

	if len(errs) > 0 {
		return msgs, fmt.Errorf("a2a/journal: recovery errors: %s", strings.Join(errs, "; "))
	}
	return msgs, nil
}

// Len returns the number of pending messages currently in the journal.
// Useful for diagnostics and health checks.
func (j *PendingJournal) Len() int {
	j.mu.Lock()
	defer j.mu.Unlock()

	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			count++
		}
	}
	return count
}

// Dir returns the directory backing this journal (for diagnostics).
func (j *PendingJournal) Dir() string { return j.dir }
