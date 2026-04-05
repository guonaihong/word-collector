package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// translateLLM translates a word using configured LLM models concurrently
func translateLLM(word string) *WordData {
	if ankiConfig == nil || len(ankiConfig.LLMModels) == 0 {
		return nil
	}

	type modelOutput struct {
		Index int
		Data  *WordData
	}

	ch := make(chan modelOutput, len(ankiConfig.LLMModels))
	var wg sync.WaitGroup

	for i, cfg := range ankiConfig.LLMModels {
		if cfg.Endpoint == "" || cfg.APIKey == "" || cfg.Model == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, c LLMModelConfig) {
			defer wg.Done()
			data := callLLMModel(word, c)
			if data != nil {
				ch <- modelOutput{Index: idx, Data: data}
			}
		}(i, cfg)
	}

	// Close channel once all goroutines finish
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect results in order
	var results []modelOutput
	for out := range ch {
		results = append(results, out)
	}

	if len(results) == 0 {
		return nil
	}

	// Use first successful result as base (respecting original config order)
	base := results[0]
	for _, r := range results {
		if r.Index < base.Index {
			base = r
		}
	}
	merged := base.Data

	// Build ModelResults from all successful models, preserving config order
	for i, cfg := range ankiConfig.LLMModels {
		for _, r := range results {
			if r.Index == i {
				html := formatModelResultHTML(r.Data)
				merged.ModelResults = append(merged.ModelResults, ModelResult{
					ModelName: cfg.Name,
					Content:   html,
				})
				break
			}
		}
	}

	return merged
}

// callLLMModel calls a single LLM model and returns parsed WordData
func callLLMModel(word string, cfg LLMModelConfig) *WordData {
	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/chat/completions"

	prompt := fmt.Sprintf(`请翻译以下英文单词/短语，给出完整的学习卡片内容。
格式要求（严格按此格式返回，每项一行，不要多余内容）：
音标: /xxx/
谐音: 用中文模拟英文发音，方便记忆
释义: 中文释义1; 中文释义2
例句: 英文例句1 (中文翻译1)
例句: 英文例句2 (中文翻译2)
易混淆: word1 - 释义1 | word2 - 释义2 | word3 - 释义3
记忆: 用词根拆解、谐音联想、字形联想、场景画面感、口诀等方式，帮助记住这个单词（可以多种方式组合，生动有趣）

重要："易混淆"部分要特别注重字形接近的单词（如 warning/warming, diary/dairy, angel/angle, quiet/quite, dessert/desert），这类只差一两个字母的词很容易读错或混淆。请列出 2-4 个字形相近的词，每个都标注中文释义和与目标词的区别。

单词: %s`, word)

	reqBody := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个英语学习助手。返回音标、中文谐音（用中文模拟英文发音，如 able → \u201c爱博\u201d，尽量接近真实发音）、中文释义、常用例句、字形相近的易混淆词、以及记忆技巧。严格按用户格式返回。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  4096,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("⚠️  LLM [%s] request marshal error: %v\n", cfg.Name, err)
		return nil
	}

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		fmt.Printf("⚠️  LLM [%s] request error: %v\n", cfg.Name, err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  LLM [%s] API error: %v\n", cfg.Name, err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("⚠️  LLM [%s] API status %d: %s\n", cfg.Name, resp.StatusCode, string(body))
		return nil
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("⚠️  LLM [%s] response parse error: %v\n", cfg.Name, err)
		return nil
	}

	if len(result.Choices) == 0 {
		return nil
	}

	msg := result.Choices[0].Message
	content := strings.TrimSpace(msg.Content)
	// If content is empty but reasoning_content has value, use reasoning_content
	if content == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		content = strings.TrimSpace(msg.ReasoningContent)
	}
	// Strip qwen3 think blocks
	closingTag := "</think" + ">"
	if idx := strings.Index(content, closingTag); idx >= 0 {
		content = strings.TrimSpace(content[idx+len(closingTag):])
	}
	fmt.Printf("🔍 LLM [%s] raw [%s]: %s\n", cfg.Name, word, content)
	return parseLLMResponse(word, content)
}

// formatModelResultHTML generates an HTML fragment for a model's full result
func formatModelResultHTML(data *WordData) string {
	var parts []string
	if data.Translation != "" {
		parts = append(parts, fmt.Sprintf("<b>%s</b>", data.Translation))
	}
	if data.Examples != "" {
		parts = append(parts, fmt.Sprintf("<div style='color:#4a9eff;font-size:0.9em;'>📖 例句</div><div style='font-size:0.85em;'>%s</div>", data.Examples))
	}
	if data.Confusables != "" {
		parts = append(parts, fmt.Sprintf("<div style='color:#ff9800;font-size:0.9em;'>⚠️ 易混淆</div><div style='font-size:0.85em;'>%s</div>", data.Confusables))
	}
	if data.MemoryAid != "" {
		parts = append(parts, fmt.Sprintf("<div style='color:#ab47bc;font-size:0.9em;'>💡 记忆技巧</div><div style='font-size:0.85em;'>%s</div>", data.MemoryAid))
	}
	return strings.Join(parts, "<br>")
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
		} else if strings.HasPrefix(line, "谐音:") || strings.HasPrefix(line, "谐音：") {
			data.CnPronunciation = strings.TrimSpace(trimPrefix(line, "谐音"))
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
