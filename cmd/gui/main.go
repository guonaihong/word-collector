package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	AnkiTag = "word-collector"
)

type WordData struct {
	Word        string `json:"word"`
	Phonetic    string `json:"phonetic"`
	Translation string `json:"translation"`
	Definition  string `json:"definition"`
	Examples    string `json:"examples"`
	Confusables string `json:"confusables"`
	MemoryAid   string `json:"memory_aid"`
}

type AppState struct {
	IsEnabled bool `json:"enabled"`
}

var (
	appState   = &AppState{IsEnabled: true}
	statusIcon *canvas.Text
	statusText *widget.Label
	enableBtn  *widget.Button
	wordEntry  *widget.Entry
	mainWindow fyne.Window
	fyneApp    fyne.App
	statusCard *fyne.Container
)

func main() {
	// Set Chinese-compatible font for Fyne (macOS system fonts)
	if os.Getenv("FYNE_FONT") == "" {
		fonts := []string{
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/STHeiti Medium.ttc",
			"/System/Library/Fonts/Supplemental/Songti.ttc",
		}
		for _, f := range fonts {
			if _, err := os.Stat(f); err == nil {
				os.Setenv("FYNE_FONT", f)
				break
			}
		}
	}

	fyneApp = app.NewWithID("com.wordcollector.gui")
	fyneApp.Settings().SetTheme(theme.DarkTheme())

	mainWindow = fyneApp.NewWindow("Word Collector")
	mainWindow.Resize(fyne.NewSize(320, 280))
	mainWindow.SetContent(buildUI())
	loadState()
	loadAnkiConfig()
	setupSystemTray()

	// Auto-install AnkiConnect plugin if not present
	if msg := ensureAnkiConnect(); msg != "" {
		fmt.Println("📦 " + msg)
		showNotification("Word Collector", msg)
	}

	// First run: prompt user to select Anki deck
	if !isAnkiConfigured() {
		go func() {
			// Wait a moment for window to appear
			time.Sleep(500 * time.Millisecond)
			showDeckSetupDialog()
		}()
	}

	// Register global hotkeys (⌃⌥⌘W: collect, ⌃⌥⌘S: toggle)
	if err := setupGlobalHotkeys(); err != nil {
		fmt.Printf("⚠️  Global hotkeys not available: %v\n", err)
		fmt.Println("Tip: Grant accessibility permission in System Settings → Privacy & Security → Accessibility")
	}

	mainWindow.SetCloseIntercept(func() { mainWindow.Hide() })
	mainWindow.ShowAndRun()

	cleanupGlobalHotkeys()
}

func buildUI() *fyne.Container {
	// 状态图标 (大号)
	statusIcon = canvas.NewText("●", color.NRGBA{R: 76, G: 175, B: 80, A: 255})
	statusIcon.TextSize = 24

	// 状态文字
	statusText = widget.NewLabelWithStyle("Enabled", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// 状态卡片
	statusCard = container.NewVBox(
		container.NewCenter(container.NewHBox(statusIcon, statusText)),
	)

	// 切换按钮
	enableBtn = widget.NewButton("⏸ Pause", toggleEnabled)
	enableBtn.Importance = widget.HighImportance

	// 单词输入
	wordEntry = widget.NewEntry()
	wordEntry.SetPlaceHolder("Enter word...")
	wordEntry.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text != "" {
			addWord(text)
			wordEntry.SetText("")
		}
	}

	addBtn := widget.NewButton("Add", func() {
		word := strings.TrimSpace(wordEntry.Text)
		if word != "" {
			addWord(word)
			wordEntry.SetText("")
		}
	})

	pasteBtn := widget.NewButton("Paste", func() {
		if word := getClipboard(); word != "" {
			wordEntry.SetText(word)
		}
	})

	ankiBtn := widget.NewButton("Anki", func() {
		exec.Command("open", "-a", "Anki").Run()
	})

	retranslateBtn := widget.NewButton("🔄 刷新翻译", func() {
		showRetranslateDialog()
	})

	settingsBtn := widget.NewButton("⚙ Settings", func() {
		showDeckSetupDialog()
	})

	// 布局
	content := container.NewVBox(
		container.NewCenter(container.NewPadded(statusCard)),
		enableBtn,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, container.NewHBox(pasteBtn, addBtn), wordEntry),
		widget.NewSeparator(),
		container.NewGridWithColumns(3, ankiBtn, retranslateBtn, settingsBtn),
	)

	return container.NewPadded(content)
}

func toggleEnabled() {
	appState.IsEnabled = !appState.IsEnabled
	updateStatus()
	saveState()
}

func updateStatus() {
	if appState.IsEnabled {
		statusIcon.Color = color.NRGBA{R: 76, G: 175, B: 80, A: 255} // Green
		statusIcon.Refresh()
		statusText.SetText("Enabled")
		enableBtn.SetText("⏸ Pause")
	} else {
		statusIcon.Color = color.NRGBA{R: 244, G: 67, B: 54, A: 255} // Red
		statusIcon.Refresh()
		statusText.SetText("Paused")
		enableBtn.SetText("▶ Enable")
	}
}

func addWord(word string) {
	if !appState.IsEnabled {
		return
	}

	wordData := fetchTranslation(word)
	if wordData.Translation == "" {
		wordData.Translation = "[Add translation]"
	}

	front, back := generateAnkiCard(wordData)

	// Ensure AnkiConnect is available, launch Anki if needed
	if !isAnkiConnectAvailable() {
		if isAnkiConnectInstalled() && !isAnkiRunning() {
			exec.Command("open", "-a", "Anki").Run()
			for i := 0; i < 16; i++ {
				time.Sleep(500 * time.Millisecond)
				if isAnkiConnectAvailable() {
					break
				}
			}
		}
	}

	preview := wordData.Translation
	if len(preview) > 40 {
		preview = preview[:40] + "..."
	}

	if !isAnkiConnectAvailable() {
		showNotification("⚠️ "+wordData.Word, "Anki 未连接，请启动 Anki")
		return
	}

	err := addToAnkiViaConnect(wordData.Word, front, back)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "duplicate") {
			showNotification("⚠️ "+wordData.Word, "已存在，跳过")
		} else {
			fmt.Printf("⚠️  AnkiConnect error: %v\n", err)
			showNotification("❌ "+wordData.Word, errMsg)
		}
		return
	}

	showNotification("✅ "+wordData.Word, preview)
}

func fetchTranslation(word string) *WordData {
	result := &WordData{Word: word, Translation: ""}

	// Use LLM if configured
	if ankiConfig != nil && ankiConfig.TranslateSource == "llm" {
		if data := translateLLM(word); data != nil && data.Translation != "" {
			return data
		}
		// Fallback to Youdao if LLM fails
	}

	if data := translateYoudao(word); data != nil && data.Translation != "" {
		return data
	}
	return result
}

func translateYoudao(word string) *WordData {
	client := &http.Client{Timeout: 5 * time.Second}
	apiUrl := "https://dict.youdao.com/jsonapi?q=" + url.QueryEscape(word)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data map[string]any
	if json.Unmarshal(body, &data) != nil {
		return nil
	}

	result := &WordData{Word: word}

	if simple, ok := data["simple"].(map[string]any); ok {
		if words, ok := simple["word"].([]any); ok && len(words) > 0 {
			if wd, ok := words[0].(map[string]any); ok {
				if p, ok := wd["usphone"].(string); ok && p != "" {
					result.Phonetic = "/" + p + "/"
				}
			}
		}
	}

	if webTrans, ok := data["web_trans"].(map[string]any); ok {
		if trans, ok := webTrans["web-translation"].([]any); ok && len(trans) > 0 {
			if first, ok := trans[0].(map[string]any); ok {
				if t, ok := first["trans"].([]any); ok && len(t) > 0 {
					var texts []string
					for i, item := range t {
						if i >= 3 {
							break
						}
						if m, ok := item.(map[string]any); ok {
							if v, ok := m["value"].(string); ok && v != "" {
								texts = append(texts, v)
							}
						}
					}
					if len(texts) > 0 {
						result.Translation = strings.Join(texts, "; ")
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

func generateAnkiCard(data *WordData) (string, string) {
	front := fmt.Sprintf("%s<br><span style='color:#666;'>%s</span>", data.Word, data.Phonetic)

	back := fmt.Sprintf("<b>%s</b>", data.Translation)
	if data.Examples != "" {
		back += fmt.Sprintf("<br><br><div style='color:#4a9eff;font-size:0.9em;'>📖 例句</div><div style='font-size:0.85em;'>%s</div>", data.Examples)
	}
	if data.Confusables != "" {
		back += fmt.Sprintf("<br><div style='color:#ff9800;font-size:0.9em;'>⚠️ 易混淆</div><div style='font-size:0.85em;'>%s</div>", data.Confusables)
	}
	if data.MemoryAid != "" {
		back += fmt.Sprintf("<br><div style='color:#ab47bc;font-size:0.9em;'>💡 记忆技巧</div><div style='font-size:0.85em;'>%s</div>", data.MemoryAid)
	}
	return front, back
}

func isAnkiConnectAvailable() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8765")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func addToAnkiViaConnect(word, front, back string) error {
	if !isAnkiConfigured() {
		return fmt.Errorf("Anki not configured, please click ⚙ Settings")
	}

	reqBody := map[string]any{
		"action":  "addNote",
		"version": 6,
		"params": map[string]any{
			"note": map[string]any{
				"deckName":  ankiConfig.DeckName,
				"modelName": ankiConfig.ModelName,
				"fields":    map[string]string{ankiConfig.FrontField: front, ankiConfig.BackField: back},
				"tags":      []string{AnkiTag},
				"options":   map[string]any{"allowDuplicate": false},
			},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	resp, err := http.Post("http://localhost:8765", "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct{ Error string }
	json.Unmarshal(body, &result)
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func getClipboard() string {
	output, err := exec.Command("pbpaste").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func showNotification(title, message string) {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	exec.Command("osascript", "-e", script).Run()
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func getStateFile() string {
	return expandPath("~/.wordcollector_state.json")
}

func saveState() {
	data, _ := json.Marshal(appState)
	os.WriteFile(getStateFile(), data, 0644)
}

func loadState() {
	if data, err := os.ReadFile(getStateFile()); err == nil {
		json.Unmarshal(data, appState)
	}
	updateStatus()
}

func setupSystemTray() {
	if desk, ok := fyneApp.(desktop.App); ok {
		menu := fyne.NewMenu("Word Collector",
			fyne.NewMenuItem("Show Window", func() {
				mainWindow.Show()
				mainWindow.RequestFocus()
			}),
			fyne.NewMenuItem("Toggle Enable", toggleEnabled),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() { fyneApp.Quit() }),
		)
		desk.SetSystemTrayMenu(menu)
		desk.SetSystemTrayIcon(theme.ComputerIcon())
	}
}
