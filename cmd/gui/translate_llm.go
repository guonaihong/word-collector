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

	prompt := fmt.Sprintf(`请翻译以下英文单词/短语，给出完整的学习卡片内容。
格式要求（严格按此格式返回，每项一行，不要多余内容）：
音标: /xxx/
释义: 中文释义1; 中文释义2
例句: 英文例句1 (中文翻译1)
例句: 英文例句2 (中文翻译2)
易混淆: word1 - 释义1 | word2 - 释义2
记忆: 用词根拆解、谐音联想、字形联想、场景画面感、口诀等方式，帮助记住这个单词（可以多种方式组合，生动有趣）

单词: %s`, word)

	reqBody := map[string]any{
		"model": ankiConfig.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个英语学习助手。返回音标、中文释义、常用例句（带中文翻译）、容易混淆的近义词/形近词、以及记忆技巧（词根拆解、谐音联想、字形画面、口诀等，越生动有趣越好）。严格按用户要求的格式返回。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  800,
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

// parseLLMResponse extracts phonetic, translation, examples and confusables from LLM response
func parseLLMResponse(word, content string) *WordData {
	data := &WordData{Word: word}

	var examples []string
	var confusables []string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "音标:") || strings.HasPrefix(line, "音标：") {
			data.Phonetic = strings.TrimSpace(trimPrefix(line, "音标"))
		} else if strings.HasPrefix(line, "释义:") || strings.HasPrefix(line, "释义：") {
			data.Translation = strings.TrimSpace(trimPrefix(line, "释义"))
		} else if strings.HasPrefix(line, "例句:") || strings.HasPrefix(line, "例句：") {
			ex := strings.TrimSpace(trimPrefix(line, "例句"))
			if ex != "" {
				examples = append(examples, ex)
			}
		} else if strings.HasPrefix(line, "易混淆:") || strings.HasPrefix(line, "易混淆：") {
			cf := strings.TrimSpace(trimPrefix(line, "易混淆"))
			if cf != "" {
				confusables = append(confusables, cf)
			}
		} else if strings.HasPrefix(line, "记忆:") || strings.HasPrefix(line, "记忆：") {
			aid := strings.TrimSpace(trimPrefix(line, "记忆"))
			if aid != "" {
				data.MemoryAid = aid
			}
		}
	}

	if len(examples) > 0 {
		data.Examples = strings.Join(examples, "<br>")
	}
	if len(confusables) > 0 {
		data.Confusables = strings.Join(confusables, "<br>")
	}

	// Fallback: if no structured format, use entire content as translation
	if data.Translation == "" {
		data.Translation = content
	}

	return data
}

// trimPrefix removes "key:" or "key：" prefix
func trimPrefix(line, key string) string {
	line = strings.TrimPrefix(line, key+":")
	line = strings.TrimPrefix(line, key+"：")
	return line
}
