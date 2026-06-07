package main

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// generateFakeWords creates n random lowercase English-looking words
func generateFakeWords(n int) []string {
	words := make([]string, n)
	for i := 0; i < n; i++ {
		l := 4 + rand.Intn(8) // 4-11 chars
		var b strings.Builder
		for j := 0; j < l; j++ {
			b.WriteByte(byte('a' + rand.Intn(26)))
		}
		words[i] = b.String()
	}
	return words
}

// realWords is a sample of common English words for realistic testing
var realWords = []string{
	"abandon", "ability", "able", "about", "above", "absent", "absorb", "abstract",
	"absurd", "abuse", "access", "accident", "account", "accuse", "achieve", "acid",
	"acoustic", "acquire", "across", "act", "action", "actor", "actress", "actual",
	"adapt", "add", "addict", "address", "adjust", "admit", "adult", "advance",
	"advice", "aerobic", "affair", "afford", "afraid", "again", "age", "agent",
	"agree", "ahead", "aim", "air", "airport", "aisle", "alarm", "album",
	"alcohol", "alert", "alien", "all", "alley", "allow", "almost", "alone",
	"alpha", "already", "also", "alter", "always", "amateur", "amazing", "among",
	"amount", "amused", "analyst", "anchor", "ancient", "anger", "angle", "angry",
	"animal", "ankle", "announce", "annual", "another", "answer", "antenna", "antique",
	"anxiety", "any", "apart", "apology", "appear", "apple", "approve", "april",
	"arch", "arctic", "area", "arena", "argue", "arm", "armed", "armor",
	"army", "around", "arrange", "arrest", "arrive", "arrow", "art", "artefact",
	"artist", "artwork", "ask", "aspect", "assault", "asset", "assist", "assume",
	"asthma", "athlete", "atom", "attack", "attend", "attitude", "attract", "auction",
	"audit", "august", "aunt", "author", "auto", "autumn", "average", "avocado",
	"avoid", "awake", "aware", "awesome", "awful", "awkward", "axis", "baby",
	"bachelor", "bacon", "badge", "bag", "balance", "balcony", "ball", "bamboo",
	"banana", "banner", "bar", "barely", "bargain", "barrel", "base", "basic",
	"basket", "battle", "beach", "bean", "beauty", "because", "become", "beef",
	"before", "begin", "behave", "behind", "believe", "below", "belt", "bench",
	"benefit", "best", "betray", "better", "between", "beyond", "bicycle", "bid",
	"bind", "biology", "bird", "birth", "bitter", "black", "blade", "blame",
	"blanket", "blast", "bleak", "bless", "blind", "blood", "blossom", "blow",
	"blue", "blur", "blush", "board", "boat", "body", "boil", "bomb",
	"bone", "bonus", "book", "boost", "border", "boring", "borrow", "boss",
	"bottom", "bounce", "box", "boy", "bracket", "brain", "brand", "brass",
	"brave", "bread", "breeze", "brick", "bridge", "brief", "bright", "bring",
	"brisk", "broccoli", "broken", "bronze", "broom", "brother", "brown", "brush",
	"bubble", "buddy", "budget", "buffalo", "build", "bulb", "bulk", "bullet",
	"bundle", "bunny", "burden", "burger", "burst", "bus", "business", "busy",
	"butter", "buyer", "buzz", "cabbage", "cabin", "cable", "cactus", "cage",
	"warning", "warming", "diary", "dairy", "angel", "angle", "quiet", "quite",
	"dessert", "desert", "accept", "except", "affect", "effect", "advice", "advise",
	"stationary", "stationery", "principle", "principal", "complement", "compliment",
}

func TestFindSimilarWords_RealWords(t *testing.T) {
	tests := []struct {
		target     string
		wantNearby []string // words that should appear in results
	}{
		{"warning", []string{"warming"}},
		{"diary", []string{"dairy"}},
		{"angel", []string{"angle"}},
		{"quiet", []string{"quite"}},
		{"dessert", []string{"desert"}},
		{"accept", []string{"except"}},
		{"advice", []string{"advise"}},
	}

	for _, tt := range tests {
		result := findSimilarWords(tt.target, realWords, 10)
		fmt.Printf("Target: %-10s -> Top10: %v\n", tt.target, result)

		for _, want := range tt.wantNearby {
			found := false
			for _, w := range result {
				if w == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("findSimilarWords(%q): expected %q in results, got %v", tt.target, want, result)
			}
		}
	}
}

func TestFindSimilarWords_CaseInsensitive(t *testing.T) {
	candidates := []string{"Apple", "APRICOT", "Cherry", "banana"}
	result := findSimilarWords("apple", candidates, 5)

	if len(result) == 0 {
		t.Fatal("expected results")
	}
	// "Apple" should be excluded (exact match), rest should be returned
	for _, w := range result {
		if w == "Apple" {
			t.Error("exact match should be excluded")
		}
	}
}

func TestFindSimilarWords_Empty(t *testing.T) {
	if r := findSimilarWords("", realWords, 10); r != nil {
		t.Errorf("expected nil for empty target, got %v", r)
	}
	if r := findSimilarWords("hello", nil, 10); r != nil {
		t.Errorf("expected nil for nil candidates, got %v", r)
	}
	if r := findSimilarWords("hello", realWords, 0); r != nil {
		t.Errorf("expected nil for topN=0, got %v", r)
	}
}

// go test -bench=. -benchmem
func BenchmarkFindSimilarWords_100(b *testing.B)  { benchFindSimilar(b, 100) }
func BenchmarkFindSimilarWords_500(b *testing.B)  { benchFindSimilar(b, 500) }
func BenchmarkFindSimilarWords_1K(b *testing.B)   { benchFindSimilar(b, 1000) }
func BenchmarkFindSimilarWords_5K(b *testing.B)   { benchFindSimilar(b, 5000) }
func BenchmarkFindSimilarWords_10K(b *testing.B)  { benchFindSimilar(b, 10000) }
func BenchmarkFindSimilarWords_50K(b *testing.B)  { benchFindSimilar(b, 50000) }
func BenchmarkFindSimilarWords_100K(b *testing.B) { benchFindSimilar(b, 100000) }

func benchFindSimilar(b *testing.B, n int) {
	words := generateFakeWords(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		findSimilarWords("abandon", words, 10)
	}
}

func BenchmarkLevenshteinDistance(b *testing.B) {
	a := []byte("stationary")
	c := []byte("stationery")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		levenshteinDistance(a, c)
	}
}
