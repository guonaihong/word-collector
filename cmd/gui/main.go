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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	AnkiTag    = "word-collector"
	OutputFile = "~/word-collector/anki_import.txt"
)

type WordData struct {
	Word        string `json:"word"`
	Phonetic    string `json:"phonetic"`
	Translation string `json:"translation"`
	Definition  string `json:"definition"`
}

type AppState struct {
	IsEnabled bool `json:"enabled"`
	WordCount int  `json:"word_count"`
}

var (
	appState   = &AppState{IsEnabled: true, WordCount: 0}
	statusIcon *canvas.Text
	statusText *widget.Label
	countLabel *widget.Label
	enableBtn  *widget.Button
	wordEntry  *widget.Entry
	mainWindow fyne.Window
	fyneApp    fyne.App
	statusCard *fyne.Container
)

func main() {
	fyneApp = app.NewWithID("com.wordcollector.gui")
	fyneApp.Settings().SetTheme(theme.DarkTheme())

	mainWindow = fyneApp.NewWindow("Word Collector")
	mainWindow.Resize(fyne.NewSize(320, 280))
	mainWindow.SetContent(buildUI())
	loadState()
	setupSystemTray()
	mainWindow.SetCloseIntercept(func() { mainWindow.Hide() })
	mainWindow.ShowAndRun()
}

func buildUI() *fyne.Container {
	// 状态图标 (大号)
	statusIcon = canvas.NewText("●", color.NRGBA{R: 76, G: 175, B: 80, A: 255})
	statusIcon.TextSize = 24

	// 状态文字
	statusText = widget.NewLabelWithStyle("Enabled", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	countLabel = widget.NewLabelWithStyle("0 words", fyne.TextAlignCenter, fyne.TextStyle{})

	// 状态卡片
	statusCard = container.NewVBox(
		container.NewCenter(container.NewHBox(statusIcon, statusText)),
		container.NewCenter(countLabel),
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

	folderBtn := widget.NewButton("Folder", func() {
		dir := expandPath("~/word-collector")
		os.MkdirAll(dir, 0755)
		exec.Command("open", dir).Run()
	})

	// 布局
	content := container.NewVBox(
		container.NewCenter(container.NewPadded(statusCard)),
		enableBtn,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, container.NewHBox(pasteBtn, addBtn), wordEntry),
		widget.NewSeparator(),
		container.NewGridWithColumns(2, ankiBtn, folderBtn),
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
	countLabel.SetText(fmt.Sprintf("%d words", appState.WordCount))
}

func addWord(word string) {
	if !appState.IsEnabled {
		dialog.ShowError(fmt.Errorf("paused"), mainWindow)
		return
	}

	wordData := fetchTranslation(word)
	if wordData.Translation == "" {
		wordData.Translation = "[Add translation]"
	}

	front, back := generateAnkiCard(wordData)

	addedToAnki := false
	if isAnkiConnectAvailable() {
		if err := addToAnkiViaConnect(wordData.Word, front, back); err == nil {
			addedToAnki = true
		}
	}

	saveToAnkiFile(front, back, wordData.Word)

	appState.WordCount++
	updateStatus()
	saveState()

	msg := fmt.Sprintf("Word: %s\nTranslation: %s", wordData.Word, wordData.Translation)
	if addedToAnki {
		showNotification("Added to Anki", msg)
	} else {
		showNotification("Saved", msg)
	}
}

func fetchTranslation(word string) *WordData {
	result := &WordData{Word: word, Translation: ""}
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
	return front, back
}

func saveToAnkiFile(front, back, _ string) string {
	outputFile := expandPath(OutputFile)
	os.MkdirAll(filepath.Dir(outputFile), 0755)
	line := fmt.Sprintf("%s\t%s\t%s\n", front, back, AnkiTag)
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return outputFile
	}
	defer f.Close()
	f.WriteString(line)
	return outputFile
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
	reqBody := map[string]any{
		"action":  "addNote",
		"version": 6,
		"params": map[string]any{
			"note": map[string]any{
				"deckName":  "Default",
				"modelName": "Basic",
				"fields":    map[string]string{"Front": front, "Back": back},
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
