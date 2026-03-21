package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	AnkiTag    = "word-collector"
	OutputFile = "~/word-collector/anki_import.txt"
)

// WordData 存储单词查询结果
type WordData struct {
	Word        string `json:"word"`
	Phonetic    string `json:"phonetic"`
	Translation string `json:"translation"`
	Definition  string `json:"definition"`
}

func main() {
	var word string
	if len(os.Args) > 1 {
		word = os.Args[1]
	} else {
		word = getSelectedText()
	}

	if word == "" {
		showNotification("Word Collector", "没有检测到选中的文本")
		fmt.Println("错误: 没有检测到选中的文本")
		fmt.Println("请选中一个英文单词后运行此脚本")
		os.Exit(1)
	}

	// 清理单词
	word = strings.TrimSpace(strings.ToLower(word))

	// 检查长度
	if len(strings.Fields(word)) > 3 {
		showNotification("Word Collector", "请选择单个单词或短词组")
		fmt.Println("错误: 请选择单个单词或短词组")
		os.Exit(1)
	}

	fmt.Printf("正在查询: %s\n", word)

	// 获取翻译
	wordData := fetchTranslation(word)

	// 生成卡片
	front, back := generateAnkiCard(wordData)

	// 尝试直接添加到 Anki
	addedToAnki := false
	if isAnkiConnectAvailable() {
		// 唤起 Anki
		launchAnki()
		// 等待 Anki 启动
		time.Sleep(500 * time.Millisecond)

		if err := addToAnkiViaConnect(wordData.Word, front, back); err == nil {
			addedToAnki = true
			fmt.Println("✅ 已直接添加到 Anki")
		} else {
			fmt.Printf("⚠️  AnkiConnect 添加失败: %v\n", err)
			fmt.Println("将保存到文件...")
		}
	} else {
		fmt.Println("⚠️  AnkiConnect 未检测到，将保存到文件")
		fmt.Println("提示: 安装 AnkiConnect 插件可实现自动导入")
		// 仍然唤起 Anki 以便用户手动导入
		launchAnki()
	}

	// 无论是否成功添加到 Anki，都保存到文件作为备份
	outputFile := saveToAnkiFile(front, back, wordData.Word)

	// 显示通知
	preview := wordData.Translation
	if len(preview) > 30 {
		preview = preview[:30] + "..."
	}

	if addedToAnki {
		showNotification("Word Collector", fmt.Sprintf("✅ 已导入 Anki: %s\n%s", wordData.Word, preview))
	} else {
		showNotification("Word Collector", fmt.Sprintf("已保存: %s\n请手动导入 Anki", wordData.Word))
	}

	fmt.Printf("✅ 成功添加单词: %s\n", wordData.Word)
	fmt.Printf("音标: %s\n", wordData.Phonetic)
	fmt.Printf("释义: %s\n", wordData.Translation)
	if !addedToAnki {
		fmt.Printf("保存位置: %s\n", outputFile)
	}
}

// getSelectedText 获取选中的文本
func getSelectedText() string {
	// 尝试从剪贴板获取
	cmd := exec.Command("pbpaste")
	output, err := cmd.Output()
	if err == nil {
		text := strings.TrimSpace(string(output))
		// 如果内容较短，可能是选中的单词
		if text != "" && len(strings.Fields(text)) <= 3 {
			return text
		}
	}

	// 尝试通过 AppleScript 获取选中的文本
	script := `
	tell application "System Events"
		keystroke "c" using {command down}
	end tell
	delay 0.2
	return the clipboard
	`
	cmd = exec.Command("osascript", "-e", script)
	output, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	return ""
}

// fetchTranslation 依次尝试多个翻译源
func fetchTranslation(word string) *WordData {
	result := &WordData{
		Word:        word,
		Phonetic:    "",
		Translation: "[需要手动添加释义]",
		Definition:  "",
	}

	// 尝试1: 有道词典
	if data := translateYoudao(word); data != nil && data.Translation != "" {
		fmt.Println("✓ 使用有道词典翻译")
		return data
	}

	// // 尝试2: 金山词霸
	// if data := translateIciba(word); data != nil && data.Translation != "" {
	// 	fmt.Println("✓ 使用金山词霸翻译")
	// 	return data
	// }

	// // 尝试3: Free Dictionary API
	// if data := translateFreeDictionary(word); data != nil {
	// 	fmt.Println("✓ 使用 Free Dictionary API")
	// 	data.Translation = "[英文释义，需手动添加中文]"
	// 	return data
	// }

	return result
}

// translateYoudao 使用有道词典 API
func translateYoudao(word string) *WordData {
	client := &http.Client{Timeout: 5 * time.Second}
	apiUrl := "https://dict.youdao.com/jsonapi?q=" + url.QueryEscape(word)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("有道翻译请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	result := &WordData{Word: word}

	// 获取音标 - 从 simple 字段
	if simple, ok := data["simple"].(map[string]any); ok {
		if words, ok := simple["word"].([]any); ok && len(words) > 0 {
			if wordData, ok := words[0].(map[string]any); ok {
				if usphone, ok := wordData["usphone"].(string); ok && usphone != "" {
					result.Phonetic = fmt.Sprintf("/%s/", usphone)
				} else if ukphone, ok := wordData["ukphone"].(string); ok && ukphone != "" {
					result.Phonetic = fmt.Sprintf("/%s/", ukphone)
				}
			}
		}
	}

	// 获取翻译 - 从 web_trans 字段
	if webTrans, ok := data["web_trans"].(map[string]any); ok {
		if translations, ok := webTrans["web-translation"].([]any); ok && len(translations) > 0 {
			if firstTrans, ok := translations[0].(map[string]any); ok {
				if trans, ok := firstTrans["trans"].([]any); ok && len(trans) > 0 {
					var transTexts []string
					for i, t := range trans {
						if i >= 3 {
							break
						}
						if tMap, ok := t.(map[string]any); ok {
							if value, ok := tMap["value"].(string); ok && value != "" {
								transTexts = append(transTexts, value)
							}
						}
					}
					if len(transTexts) > 0 {
						result.Translation = strings.Join(transTexts, "; ")
						result.Definition = strings.Join(transTexts, "\n")
					}
				}
			}
		}
	}

	// 获取更多释义 - 从 syno 字段
	if syno, ok := data["syno"].(map[string]any); ok {
		if synos, ok := syno["synos"].([]any); ok && len(synos) > 0 {
			if firstSyno, ok := synos[0].(map[string]any); ok {
				if s, ok := firstSyno["syno"].(map[string]any); ok {
					if tran, ok := s["tran"].(string); ok && tran != "" {
						if result.Translation == "" {
							result.Translation = tran
						}
						result.Definition = tran
					}
				}
			}
		}
	}

	if result.Translation == "" {
		return nil
	}

	return result
}

// generateAnkiCard 生成 Anki 卡片格式
func generateAnkiCard(data *WordData) (string, string) {
	// 正面：英文单词 + 音标
	front := fmt.Sprintf("%s<br><span style='color:#666;font-size:14px;'>%s</span>",
		data.Word, data.Phonetic)

	// 背面：中文释义
	back := fmt.Sprintf("<b>%s</b>", data.Translation)
	if data.Definition != "" && len(data.Definition) > len(data.Translation) {
		back = fmt.Sprintf("<b>%s</b><br><br><div style='text-align:left;font-size:14px;'>",
			data.Translation)
		lines := strings.Split(data.Definition, "\n")
		for _, line := range lines {
			back += line + "<br>"
		}
		back += "</div>"
	}

	return front, back
}

// saveToAnkiFile 保存到 Anki 导入文件
func saveToAnkiFile(front, back, _ string) string {
	outputFile := expandPath(OutputFile)

	// 确保目录存在
	dir := filepath.Dir(outputFile)
	os.MkdirAll(dir, 0755)

	// 格式: 正面 背面 标签 (Tab 分隔)
	line := fmt.Sprintf("%s\t%s\t%s\n", front, back, AnkiTag)

	// 追加到文件
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return ""
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		fmt.Printf("写入文件失败: %v\n", err)
	}

	return outputFile
}

// showNotification 显示 macOS 通知
func showNotification(title, message string) {
	script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "Glass"`,
		message, title)
	cmd := exec.Command("osascript", "-e", script)
	cmd.Run()
}

// expandPath 展开 ~ 到用户主目录
func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	return path
}

// launchAnki 唤起 Anki 应用
func launchAnki() {
	// 检查 Anki 是否已在运行
	script := `tell application "System Events" to (name of processes) contains "Anki"`
	cmd := exec.Command("osascript", "-e", script)
	output, _ := cmd.Output()

	if strings.TrimSpace(string(output)) == "true" {
		// Anki 已在运行，激活它
		script = `tell application "Anki" to activate`
	} else {
		// Anki 未运行，启动它
		script = `tell application "Anki" to launch`
	}

	cmd = exec.Command("osascript", "-e", script)
	cmd.Run()
}

// isAnkiConnectAvailable 检查 AnkiConnect 是否可用
func isAnkiConnectAvailable() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8765")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// addToAnkiViaConnect 通过 AnkiConnect API 添加卡片
func addToAnkiViaConnect(_ /* word */, front, back string) error {
	// 构建请求体
	requestBody := map[string]any{
		"action":  "addNote",
		"version": 6,
		"params": map[string]any{
			"note": map[string]any{
				"deckName":  "系统默认",
				"modelName": "问答题",
				"fields": map[string]string{
					"正面": front,
					"背面": back,
				},
				"tags": []string{AnkiTag},
				"options": map[string]any{
					"allowDuplicate": false,
					"duplicateScope": "deck",
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// 发送请求
	resp, err := http.Post("http://localhost:8765", "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("connect to Anki: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// 解析响应
	var result struct {
		Result any    `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if result.Error != "" {
		// 如果是重复卡片，不算错误
		if strings.Contains(result.Error, "duplicate") {
			fmt.Println("⚠️  卡片已存在，跳过")
			return nil
		}
		return fmt.Errorf("anki error: %s", result.Error)
	}

	fmt.Printf("✓ Anki 卡片 ID: %v\n", result.Result)
	return nil
}
