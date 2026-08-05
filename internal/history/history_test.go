package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestHistory(t *testing.T) *History {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	t.Cleanup(func() { os.Unsetenv("APPDATA") })
	return New()
}

func TestAddSingleEntry(t *testing.T) {
	h := newTestHistory(t)
	h.Add("Hello world")

	entries := h.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Text != "Hello world" {
		t.Errorf("expected text %q, got %q", "Hello world", entries[0].Text)
	}
	if entries[0].WordCount != 2 {
		t.Errorf("expected word count 2, got %d", entries[0].WordCount)
	}
}

func TestAddMultipleEntries(t *testing.T) {
	h := newTestHistory(t)
	for i := 0; i < 50; i++ {
		h.Add("test entry " + string(rune('a'+i%26)))
	}

	entries := h.GetAll()
	if len(entries) != 50 {
		t.Errorf("expected 50 entries, got %d", len(entries))
	}
}

func TestMaxEntriesEviction(t *testing.T) {
	h := newTestHistory(t)
	for i := 0; i <= maxEntries; i++ {
		h.Add("entry")
	}

	entries := h.GetAll()
	if len(entries) > maxEntries {
		t.Errorf("expected at most %d entries, got %d", maxEntries, len(entries))
	}
	if len(entries) != maxEntries {
		t.Errorf("expected exactly %d entries after overflow, got %d", maxEntries, len(entries))
	}
}

func TestSearchBySubstring(t *testing.T) {
	h := newTestHistory(t)
	h.Add("Hello world")
	h.Add("Goodbye world")
	h.Add("Random text")

	results := h.Search("ell")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'ell', got %d", len(results))
	}
	if len(results) > 0 && results[0].Text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", results[0].Text)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	h := newTestHistory(t)
	h.Add("Hello World")
	h.Add("hello world")

	results := h.Search("HELLO")
	if len(results) != 2 {
		t.Errorf("expected 2 results for case-insensitive 'HELLO', got %d", len(results))
	}
}

func TestSearchNoMatch(t *testing.T) {
	h := newTestHistory(t)
	h.Add("Hello world")
	h.Add("Goodbye world")

	results := h.Search("xyz123")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchEmptyHistory(t *testing.T) {
	h := newTestHistory(t)
	results := h.Search("anything")
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty history, got %d", len(results))
	}
}

func TestClearHistory(t *testing.T) {
	h := newTestHistory(t)
	h.Add("Hello")
	h.Add("World")

	h.Clear()

	entries := h.GetAll()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	t.Cleanup(func() { os.Unsetenv("APPDATA") })

	// Create and add entries
	h1 := New()
	h1.Add("Persisted text")
	h1.Add("Another entry")

	// Verify file was created
	histPath := filepath.Join(dir, "Yappie", "history.json")
	if _, err := os.Stat(histPath); err != nil {
		t.Fatalf("history file not created: %v", err)
	}

	// Create a new History instance — should load from disk
	h2 := New()
	entries := h2.GetAll()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries loaded from disk, got %d", len(entries))
	}
	if entries[0].Text != "Persisted text" {
		t.Errorf("expected first entry %q, got %q", "Persisted text", entries[0].Text)
	}
}

func TestEmptyHistoryLoad(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	t.Cleanup(func() { os.Unsetenv("APPDATA") })

	// No history file exists yet
	h := New()
	entries := h.GetAll()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from non-existent file, got %d", len(entries))
	}
}

func TestWordCountInEntry(t *testing.T) {
	h := newTestHistory(t)
	h.Add("one two three four five")

	entries := h.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].WordCount != 5 {
		t.Errorf("expected word count 5, got %d", entries[0].WordCount)
	}
}

func TestWordCountWithMultipleSpaces(t *testing.T) {
	h := newTestHistory(t)
	text := "one  two   three"
	h.Add(text)

	entries := h.GetAll()
	// strings.Fields splits on whitespace, so multiple spaces should still give 3 words
	if entries[0].WordCount != 3 {
		// Verify by counting manually
		manualCount := len(strings.Fields(text))
		if entries[0].WordCount != manualCount {
			t.Errorf("expected word count %d, got %d", manualCount, entries[0].WordCount)
		}
	}
}