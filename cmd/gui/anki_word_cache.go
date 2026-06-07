package main

import (
	"sync"
	"time"
)

const ankiCacheTTL = 5 * time.Minute

// ankiWordCache caches the word list from the user's Anki deck
var ankiWordCache struct {
	mu       sync.RWMutex
	words    []string
	loadedAt time.Time
}

// getAnkiWords returns the cached word list, refreshing if expired.
// Returns nil if Anki is unavailable or the deck is empty.
func getAnkiWords() []string {
	ankiWordCache.mu.RLock()
	if ankiWordCache.words != nil && time.Since(ankiWordCache.loadedAt) < ankiCacheTTL {
		words := ankiWordCache.words
		ankiWordCache.mu.RUnlock()
		return words
	}
	ankiWordCache.mu.RUnlock()

	// Need to reload
	if err := loadAnkiWords(); err != nil {
		return nil
	}

	ankiWordCache.mu.RLock()
	words := ankiWordCache.words
	ankiWordCache.mu.RUnlock()
	return words
}

// loadAnkiWords fetches all words from the configured Anki deck and updates the cache
func loadAnkiWords() error {
	if !isAnkiConfigured() {
		return nil
	}

	ids, err := findNotesInDeck(ankiConfig.DeckName)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		ankiWordCache.mu.Lock()
		ankiWordCache.words = nil
		ankiWordCache.loadedAt = time.Now()
		ankiWordCache.mu.Unlock()
		return nil
	}

	notes, err := fetchNotesInfo(ids)
	if err != nil {
		return err
	}

	var words []string
	for _, n := range notes {
		if front, ok := n.Fields[ankiConfig.FrontField]; ok {
			w := stripHTML(front)
			if w != "" {
				words = append(words, w)
			}
		}
	}

	ankiWordCache.mu.Lock()
	ankiWordCache.words = words
	ankiWordCache.loadedAt = time.Now()
	ankiWordCache.mu.Unlock()
	return nil
}

// invalidateAnkiCache forces a reload on next access
func invalidateAnkiCache() {
	ankiWordCache.mu.Lock()
	ankiWordCache.words = nil
	ankiWordCache.loadedAt = time.Time{}
	ankiWordCache.mu.Unlock()
}
