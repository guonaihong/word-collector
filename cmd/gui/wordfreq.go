package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ledongthuc/pdf"
)

var wordRegex = regexp.MustCompile(`[a-zA-Z]+`)
// WordFreqEntry represents one word and its frequency across all PDFs
type WordFreqEntry struct {
	Word     string   `json:"word"`
	Count    int      `json:"count"`
	Sources  []string `json:"sources"`
	Imported bool     `json:"imported"`
}

// WordFreqStore is the top-level persistence structure
type WordFreqStore struct {
	Files     []string          `json:"files"`
	FileNames map[string]string `json:"file_names"`
	Words     []WordFreqEntry   `json:"words"`
}

// PDFProcessResult holds the result of processing a PDF file
type PDFProcessResult struct {
	TotalPages   int
	TextLength   int
	UniqueWords  int
	TotalMatches int
	UsedOCR      bool
}

func getWordFreqFile() string {
	return expandPath("~/.wordcollector_wordfreq.json")
}

func loadWordFreqStore() *WordFreqStore {
	store := &WordFreqStore{
		FileNames: make(map[string]string),
	}
	data, err := os.ReadFile(getWordFreqFile())
	if err != nil {
		return store
	}
	json.Unmarshal(data, store)
	if store.FileNames == nil {
		store.FileNames = make(map[string]string)
	}
	return store
}

func saveWordFreqStore(s *WordFreqStore) {
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(getWordFreqFile(), data, 0644)
}

// extractTextFromPDF extracts all text content from a PDF file.
// Returns the extracted text and process result.
func extractTextFromPDF(path string, progressCB func(page, total int)) (string, *PDFProcessResult, error) {
	// Method 1: Try pdftotext (poppler-based, most reliable for text PDFs)
	if text, err := extractTextViaPdftotext(path); err == nil && len(strings.TrimSpace(text)) > 50 {
		return text, &PDFProcessResult{TextLength: len(text)}, nil
	}

	// Method 2: Try ledongthuc/pdf library
	text, err := extractTextViaGoPDF(path)
	if err == nil && len(strings.TrimSpace(text)) > 50 {
		return text, &PDFProcessResult{TextLength: len(text)}, nil
	}

	// Method 3: OCR via vision model (for scanned PDFs)
	ocrText, totalPages, ocrErr := extractTextViaOCR(path, progressCB)
	if ocrErr != nil {
		return "", nil, fmt.Errorf("无法提取文本: %w", ocrErr)
	}
	if len(strings.TrimSpace(ocrText)) == 0 {
		return "", nil, fmt.Errorf("无法提取文本（可能是空白 PDF）")
	}
	return ocrText, &PDFProcessResult{
		TextLength: len(ocrText),
		TotalPages: totalPages,
		UsedOCR:    true,
	}, nil
}

// extractTextViaPdftotext uses the pdftotext CLI tool (poppler) to extract text
func extractTextViaPdftotext(path string) (string, error) {
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, "-")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// extractTextViaGoPDF uses the Go PDF library to extract text
func extractTextViaGoPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Try Reader-level GetPlainText
	textReader, err := r.GetPlainText()
	if err == nil && textReader != nil {
		data, readErr := io.ReadAll(textReader)
		if readErr == nil && len(data) > 0 {
			return string(data), nil
		}
	}

	// Fallback to page-by-page extraction
	var buf strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString(" ")
	}
	return buf.String(), nil
}

// getPDFPageCount returns the number of pages in a PDF
func getPDFPageCount(path string) int {
	cmd := exec.Command("pdfinfo", path)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			var count int
			fmt.Sscanf(strings.TrimPrefix(line, "Pages:"), "%d", &count)
			return count
		}
	}
	return 0
}

// pdfPageToImage converts a single PDF page to a PNG image
func pdfPageToImage(pdfPath string, page int) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "pdf2img")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	prefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command("pdftoppm", "-png", "-f", fmt.Sprintf("%d", page), "-l", fmt.Sprintf("%d", page), "-r", "150", pdfPath, prefix)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %w", err)
	}

	matches, _ := filepath.Glob(filepath.Join(tmpDir, "page-*.png"))
	if len(matches) == 0 {
		return nil, fmt.Errorf("no image generated for page %d", page)
	}

	return os.ReadFile(matches[0])
}

// extractTextViaOCR uses a vision model to OCR all pages of a scanned PDF
func extractTextViaOCR(path string, progressCB func(page, total int)) (string, int, error) {
	totalPages := getPDFPageCount(path)
	if totalPages == 0 {
		return "", 0, fmt.Errorf("无法获取 PDF 页数")
	}

	var allText strings.Builder
	for page := 1; page <= totalPages; page++ {
		if progressCB != nil {
			progressCB(page, totalPages)
		}

		imageData, err := pdfPageToImage(path, page)
		if err != nil {
			continue
		}

		text, err := ocrPage(imageData)
		if err != nil {
			continue
		}
		allText.WriteString(text)
		allText.WriteString(" ")
	}

	return allText.String(), totalPages, nil
}

// ocrFailureTracker tracks consecutive failures per endpoint for circuit breaking
type ocrFailureTracker struct {
	mu        sync.Mutex
	failures  map[string]int   // endpoint -> consecutive failure count
	cooldown  map[string]time.Time // endpoint -> time until which it's blocked
	window    int               // consecutive failures before blocking
	blockDur  time.Duration     // how long to block after threshold
}

var ocrFailures = &ocrFailureTracker{
	failures: make(map[string]int),
	cooldown: make(map[string]time.Time),
	window:   3,
	blockDur: 5 * time.Minute,
}

// isBlocked returns true if the endpoint has too many recent failures
func (t *ocrFailureTracker) isBlocked(endpoint string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if until, ok := t.cooldown[endpoint]; ok && time.Now().Before(until) {
		return true
	}
	delete(t.cooldown, endpoint)
	return false
}

// recordFailure increments failure count; blocks endpoint if threshold reached
func (t *ocrFailureTracker) recordFailure(endpoint string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[endpoint]++
	if t.failures[endpoint] >= t.window {
		t.cooldown[endpoint] = time.Now().Add(t.blockDur)
		delete(t.failures, endpoint)
		fmt.Printf("🔌 OCR endpoint blocked for %v: %s\n", t.blockDur, endpoint)
	}
}

// recordSuccess clears failure count for the endpoint
func (t *ocrFailureTracker) recordSuccess(endpoint string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, endpoint)
	delete(t.cooldown, endpoint)
}

// ocrPage sends a page image to the vision model for OCR
func ocrPage(imageData []byte) (string, error) {
	// Try Claude/Anthropic vision API first (from ~/.claude/settings.json)
	if claude := loadClaudeSettingsBackend(); claude != nil {
		ep := claude.Endpoint
		if !ocrFailures.isBlocked(ep) {
			text, err := ocrPageAnthropic(imageData, claude)
			if err == nil {
				ocrFailures.recordSuccess(ep)
				return text, nil
			}
			ocrFailures.recordFailure(ep)
			fmt.Printf("⚠️  Claude OCR failed, trying next: %v\n", err)
		}
	}

	// Try GUI-configured models with vision support
	if ankiConfig != nil {
		for _, m := range ankiConfig.LLMModels {
			if m.Endpoint == "" || m.APIKey == "" || m.Model == "" {
				continue
			}
			if ocrFailures.isBlocked(m.Endpoint) {
				continue
			}
			if m.Provider == "anthropic" {
				text, err := ocrPageAnthropic(imageData, &m)
				if err == nil {
					ocrFailures.recordSuccess(m.Endpoint)
					return text, nil
				}
				ocrFailures.recordFailure(m.Endpoint)
				fmt.Printf("⚠️  [%s] OCR failed: %v\n", m.Name, err)
				continue
			}
			text, err := ocrPageOpenAI(imageData, m)
			if err == nil {
				ocrFailures.recordSuccess(m.Endpoint)
				return text, nil
			}
			ocrFailures.recordFailure(m.Endpoint)
			fmt.Printf("⚠️  [%s] OCR failed: %v\n", m.Name, err)
		}
	}

	// Fallback: auto-discover LM Studio
	endpoint := findLMStudioEndpoint()
	if endpoint != "" && !ocrFailures.isBlocked(endpoint) {
		text, err := ocrPageOpenAIEndpoint(imageData, endpoint, "paddleocr-vl@4bit")
		if err == nil {
			ocrFailures.recordSuccess(endpoint)
			return text, nil
		}
		ocrFailures.recordFailure(endpoint)
	}

	return "", fmt.Errorf("未找到可用的 OCR 模型，请在 ~/.claude/settings.json 或设置中配置模型")
}

// ocrPageOpenAI sends a page image to an OpenAI-compatible vision model
func ocrPageOpenAI(imageData []byte, cfg LLMModelConfig) (string, error) {
	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/chat/completions"
	return ocrPageOpenAIEndpoint(imageData, endpoint, cfg.Model)
}

// ocrPageOpenAIEndpoint sends a page image to an OpenAI-compatible vision endpoint
func ocrPageOpenAIEndpoint(imageData []byte, endpoint, model string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:image/png;base64,%s", b64),
						},
					},
					{
						"type": "text",
						"text": "OCR all the English text in this image. Output only the extracted text, preserving the original text as much as possible. Do not add any explanation.",
					},
				},
			},
		},
		"max_tokens":  4096,
		"temperature": 0.1,
	}

	jsonBody, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("OCR API 调用失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 OCR 响应失败: %w", err)
	}
	if result.Error.Message != "" {
		return "", fmt.Errorf("OCR API 错误: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OCR 无返回结果")
	}

	content := result.Choices[0].Message.Content
	content = stripThinkTags(content)
	return strings.TrimSpace(content), nil
}

// ocrPageAnthropic sends a page image to the Anthropic Messages API for OCR
func ocrPageAnthropic(imageData []byte, cfg *LLMModelConfig) (string, error) {
	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/v1/messages"
	b64 := base64.StdEncoding.EncodeToString(imageData)

	payload := map[string]any{
		"model":      cfg.Model,
		"max_tokens": 4096,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/png",
							"data":       b64,
						},
					},
					{
						"type": "text",
						"text": "OCR all the English text in this image. Output only the extracted text, preserving the original text as much as possible. Do not add any explanation.",
					},
				},
			},
		},
	}

	jsonBody, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("OCR request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OCR API 调用失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OCR API status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 OCR 响应失败: %w", err)
	}
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		return "", fmt.Errorf("OCR 无返回结果")
	}

	content := stripThinkTags(result.Content[0].Text)
	return strings.TrimSpace(content), nil
}

// findLMStudioEndpoint auto-discovers a running LM Studio instance
func findLMStudioEndpoint() string {
	endpoints := []string{
		"http://localhost:1234/v1/chat/completions",
		"http://127.0.0.1:1234/v1/chat/completions",
	}
	for _, ep := range endpoints {
		client := &http.Client{Timeout: 2 * time.Second}
		baseURL := strings.TrimSuffix(strings.TrimSuffix(ep, "/chat/completions"), "/v1")
		resp, err := client.Get(baseURL + "/v1/models")
		if err == nil {
			resp.Body.Close()
			return ep
		}
	}
	return ""
}

// stripThinkTags removes <think...</think tags from model output
func stripThinkTags(s string) string {
	for {
		start := strings.Index(s, "<think")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think")
		if end == -1 {
			s = s[:start]
			break
		}
		s = s[:start] + s[end+len("</think"):]
		if idx := strings.Index(s[start:], ">"); idx != -1 {
			s = s[:start+idx] + s[start+idx+1:]
		}
	}
	return strings.TrimSpace(s)
}

// tokenizeAndCount extracts English words from text and counts their frequency
func tokenizeAndCount(text string) map[string]int {
	counts := make(map[string]int)
	matches := wordRegex.FindAllString(text, -1)
	for _, m := range matches {
		word := strings.ToLower(m)
		if len(word) < 3 {
			continue
		}
		if isStopWord(word) {
			continue
		}
		counts[word]++
	}
	return counts
}

// processPDF parses a PDF, extracts words, and merges results into the store
func processPDF(path string, store *WordFreqStore, progressCB func(page, total int)) (*PDFProcessResult, error) {
	text, result, err := extractTextFromPDF(path, progressCB)
	if err != nil {
		return nil, err
	}

	counts := tokenizeAndCount(text)
	name := filepath.Base(path)

	totalMatches := 0
	for _, c := range counts {
		totalMatches += c
	}

	// Build lookup map from existing words
	wordMap := make(map[string]*WordFreqEntry)
	for i := range store.Words {
		wordMap[store.Words[i].Word] = &store.Words[i]
	}

	// Merge new counts
	for word, count := range counts {
		if entry, ok := wordMap[word]; ok {
			entry.Count += count
			found := false
			for _, s := range entry.Sources {
				if s == name {
					found = true
					break
				}
			}
			if !found {
				entry.Sources = append(entry.Sources, name)
			}
		} else {
			store.Words = append(store.Words, WordFreqEntry{
				Word:    word,
				Count:   count,
				Sources: []string{name},
			})
		}
	}

	// Sort by count descending
	sort.Slice(store.Words, func(i, j int) bool {
		return store.Words[i].Count > store.Words[j].Count
	})

	// Track file
	found := false
	for _, f := range store.Files {
		if f == path {
			found = true
			break
		}
	}
	if !found {
		store.Files = append(store.Files, path)
	}
	store.FileNames[path] = name

	result.UniqueWords = len(counts)
	result.TotalMatches = totalMatches
	return result, nil
}

// removePDF removes a PDF's contribution from the store
func removePDF(path string, store *WordFreqStore) {
	name := filepath.Base(path)

	var newFiles []string
	for _, f := range store.Files {
		if f != path {
			newFiles = append(newFiles, f)
		}
	}
	store.Files = newFiles
	delete(store.FileNames, path)

	var newWords []WordFreqEntry
	for i := range store.Words {
		entry := &store.Words[i]
		var newSources []string
		for _, s := range entry.Sources {
			if s != name {
				newSources = append(newSources, s)
			}
		}

		if len(newSources) == 0 {
			continue
		}

		entry.Sources = newSources
		newWords = append(newWords, *entry)
	}
	store.Words = newWords
}

// getWordsAlreadyInAnki returns a set of words already present in the configured Anki deck
func getWordsAlreadyInAnki() (map[string]bool, error) {
	if !isAnkiConfigured() {
		return nil, fmt.Errorf("Anki not configured")
	}

	ids, err := findNotesInDeck(ankiConfig.DeckName)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}

	notes, err := fetchNotesInfo(ids)
	if err != nil {
		return nil, err
	}

	words := make(map[string]bool)
	for _, n := range notes {
		if front, ok := n.Fields[ankiConfig.FrontField]; ok {
			w := stripHTML(front)
			if w != "" {
				words[strings.ToLower(w)] = true
			}
		}
	}
	return words, nil
}

// importWordsToAnki imports a list of words into Anki with concurrent translation
func importWordsToAnki(words []string, deckName string, cb *translateCallback) (int64, int64) {
	if len(words) == 0 {
		return 0, 0
	}

	invalidateAnkiCache()

	var success, failed, done int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	taskCh := make(chan string, len(words))

	for _, w := range words {
		taskCh <- w
	}
	close(taskCh)

	workers := 4
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for word := range taskCh {
				wordData := fetchTranslation(word)
				if wordData.Translation == "" {
					wordData.Translation = "[Add translation]"
				}

				front, back := generateAnkiCard(wordData)

				err := addToAnkiViaConnect(wordData.Word, front, back, deckName)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					atomic.AddInt64(&done, 1)
					mu.Lock()
					if strings.Contains(err.Error(), "duplicate") {
						cb.OnLog(fmt.Sprintf("⚠️ %s - 已存在，跳过", word))
					} else {
						cb.OnLog(fmt.Sprintf("❌ %s - 失败: %v", word, err))
					}
					mu.Unlock()
					continue
				}

				atomic.AddInt64(&success, 1)
				cur := atomic.AddInt64(&done, 1)
				mu.Lock()
				cb.OnLog(fmt.Sprintf("✅ %s → %s", word, wordData.Translation))
				cb.OnProgress(cur, int64(len(words)))
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return success, failed
}
