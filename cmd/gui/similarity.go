package main

import "sort"

// levenshteinDistance computes the edit distance between two strings.
// Operates on ASCII-lowercased bytes for speed.
func levenshteinDistance(a, b []byte) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use a single slice with two-row swapping
	buf := make([]int, 2*(lb+1))
	prev := buf[:lb+1]
	curr := buf[lb+1:]

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		ai := a[i-1]
		for j := 1; j <= lb; j++ {
			cost := 1
			if ai == b[j-1] {
				cost = 0
			}
			d := prev[j] + 1
			if c := curr[j-1] + 1; c < d {
				d = c
			}
			if c := prev[j-1] + cost; c < d {
				d = c
			}
			curr[j] = d
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// toLowerASCII lowercases ASCII letters in-place, returns a slice of the input buffer.
func toLowerASCII(w string, buf []byte) []byte {
	n := len(w)
	out := buf[:n]
	for i := 0; i < n; i++ {
		c := w[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

// similarWord holds a candidate word with its normalized distance score
type similarWord struct {
	word  string
	score float64
}

// findSimilarWords returns the top N words most similar to target.
// Uses normalized Levenshtein distance for fair comparison across word lengths.
func findSimilarWords(target string, candidates []string, topN int) []string {
	if target == "" || len(candidates) == 0 || topN <= 0 {
		return nil
	}

	// Pre-allocate buffers for lowercasing
	targetBuf := make([]byte, len(target))
	tLower := toLowerASCII(target, targetBuf)
	tLen := len(tLower)

	// Collect scored candidates (pre-allocate with capacity)
	scored := make([]similarWord, 0, min(len(candidates), 64))
	candBuf := make([]byte, 256) // reusable buffer for candidate lowercasing

	for _, w := range candidates {
		if len(w) == 0 {
			continue
		}
		// Grow candBuf if needed
		if len(w) > cap(candBuf) {
			candBuf = make([]byte, len(w)+64)
		}
		nw := toLowerASCII(w, candBuf)

		// Skip exact match
		if string(nw) == string(tLower) {
			continue
		}

		d := levenshteinDistance(tLower, nw)
		maxLen := tLen
		if len(nw) > maxLen {
			maxLen = len(nw)
		}

		scored = append(scored, similarWord{word: w, score: float64(d) / float64(maxLen)})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	if len(scored) > topN {
		scored = scored[:topN]
	}

	result := make([]string, len(scored))
	for i, s := range scored {
		result[i] = s.word
	}
	return result
}
