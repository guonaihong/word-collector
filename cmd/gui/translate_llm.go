package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// translateLLM translates a word using an OpenAI-compatible LLM API (Kimi, DeepSeek, etc.)
func translateLLM(word string) *WordData {
	if ankiConfig == nil || ankiConfig.LLMEndpoint == "" || ankiConfig.LLMAPIKey == "" || ankiConfig.LLMModel == "" {
		return nil
	}

	endpoint := strings.TrimRight(ankiConfig.LLMEndpoint, "/") + "/chat/completions"

	prompt := fmt.Sprintf(`请翻译以下英文单词/短语为中文，给出简洁的翻译结果。
格式要求（严格按此格式返回，不要多余内容）：
音标: /xxx/
释义: 中文释义1; 中文释义2

单词: %s`, word)

	reqBody := map[string]any{
		"model": ankiConfig.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个英中翻译助手，只返回音标和简洁的中文释义，不要解释。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  200,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("⚠️  LLM request marshal error: %v\n", err)
		return nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		fmt.Printf("⚠️  LLM request error: %v\n", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ankiConfig.LLMAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  LLM API error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("⚠️  LLM API status %d: %s\n", resp.StatusCode, string(body))
		return nil
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("⚠️  LLM response parse error: %v\n", err)
		return nil
	}

	if len(result.Choices) == 0 {
		return nil
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	return parseLLMResponse(word, content)
}

// parseLLMResponse extracts phonetic and translation from LLM response
func parseLLMResponse(word, content string) *WordData {
	data := &WordData{Word: word}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "音标:") || strings.HasPrefix(line, "音标：") {
			data.Phonetic = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "音标:"), "音标："))
		} else if strings.HasPrefix(line, "释义:") || strings.HasPrefix(line, "释义：") {
			data.Translation = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "释义:"), "释义："))
		}
	}

	// Fallback: if no structured format, use entire content as translation
	if data.Translation == "" {
		data.Translation = content
	}

	return data
}
