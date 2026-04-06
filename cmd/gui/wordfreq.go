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

// ocrPage sends a page image to the vision model for OCR
func ocrPage(imageData []byte) (string, error) {
	endpoint := findOCREndpoint()
	if endpoint == "" {
		return "", fmt.Errorf("未找到可用的 OCR 模型，请启动 LM Studio 并加载 paddleocr-vl 模型")
	}

	b64 := base64.StdEncoding.EncodeToString(imageData)

	payload := map[string]any{
		"model": "paddleocr-vl@4bit",
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

// findOCREndpoint finds an available LM Studio endpoint for OCR
func findOCREndpoint() string {
	// Try configured LLM endpoints first
	if ankiConfig != nil {
		for _, m := range ankiConfig.LLMModels {
			if strings.Contains(m.Endpoint, "1234") || strings.Contains(m.Endpoint, "lmstudio") {
				return m.Endpoint + "/chat/completions"
			}
		}
	}

	// Try common LM Studio address
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
func importWordsToAnki(words []string, cb *translateCallback) (int64, int64) {
	if len(words) == 0 {
		return 0, 0
	}

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

				err := addToAnkiViaConnect(wordData.Word, front, back)
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
