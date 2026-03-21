package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]+>`)

// stripHTML removes all HTML tags from a string
func stripHTML(s string) string {
	return strings.TrimSpace(htmlTagRegex.ReplaceAllString(s, ""))
}

// showRetranslateDialog shows the re-translate window
func showRetranslateDialog() {
	if !isAnkiConnectAvailable() {
		dialog.ShowInformation("Anki 未连接", "请先启动 Anki", mainWindow)
		return
	}

	decks, err := fetchDeckNames()
	if err != nil || len(decks) == 0 {
		dialog.ShowError(fmt.Errorf("获取牌组失败"), mainWindow)
		return
	}

	selectedDeck := decks[0]
	if ankiConfig.DeckName != "" {
		for _, d := range decks {
			if d == ankiConfig.DeckName {
				selectedDeck = d
				break
			}
		}
	}

	win := fyneApp.NewWindow("🔄 刷新翻译")
	win.Resize(fyne.NewSize(480, 350))

	deckSelect := widget.NewSelect(decks, func(s string) { selectedDeck = s })
	deckSelect.SetSelected(selectedDeck)

	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	logText := widget.NewMultiLineEntry()
	logText.Wrapping = fyne.TextWrapWord
	logText.Disable()
	logText.SetMinRowsVisible(6)

	var running bool

	startBtn := widget.NewButton("开始刷新", nil)
	startBtn.Importance = widget.HighImportance

	startBtn.OnTapped = func() {
		if running {
			return
		}
		running = true
		startBtn.Disable()
		progressBar.Show()
		progressBar.SetValue(0)
		logText.SetText("")
		statusLabel.SetText("正在获取卡片...")

		go func() {
			defer func() {
				running = false
				startBtn.Enable()
			}()

			appendLog := func(msg string) {
				logText.SetText(logText.Text + msg + "\n")
				logText.CursorRow = len(strings.Split(logText.Text, "\n")) - 1
			}

			// Find all notes in the selected deck
			noteIDs, err := findNotesInDeck(selectedDeck)
			if err != nil {
				statusLabel.SetText(fmt.Sprintf("❌ 查询失败: %v", err))
				return
			}
			if len(noteIDs) == 0 {
				statusLabel.SetText("牌组中没有卡片")
				return
			}

			statusLabel.SetText(fmt.Sprintf("找到 %d 张卡片，开始翻译...", len(noteIDs)))
			appendLog(fmt.Sprintf("牌组: %s, 共 %d 张卡片", selectedDeck, len(noteIDs)))

			frontField := ankiConfig.FrontField
			backField := ankiConfig.BackField
			if frontField == "" || backField == "" {
				statusLabel.SetText("❌ 请先在设置中配置 Anki 字段")
				return
			}

			// Process in batches of 10
			updated := 0
			skipped := 0
			failed := 0
			batchSize := 10

			for i := 0; i < len(noteIDs); i += batchSize {
				end := i + batchSize
				if end > len(noteIDs) {
					end = len(noteIDs)
				}
				batch := noteIDs[i:end]

				notes, err := fetchNotesInfo(batch)
				if err != nil {
					appendLog(fmt.Sprintf("⚠️ 批量获取失败: %v", err))
					failed += len(batch)
					continue
				}

				for _, note := range notes {
					frontVal, ok := note.Fields[frontField]
					if !ok || frontVal == "" {
						skipped++
						continue
					}

					// Extract the word from front field (strip HTML)
					word := stripHTML(frontVal)
					if word == "" {
						skipped++
						continue
					}

					// Re-translate
					wordData := fetchTranslation(word)
					if wordData.Translation == "" || wordData.Translation == "[Add translation]" {
						appendLog(fmt.Sprintf("⚠️ %s - 翻译失败", word))
						failed++
						continue
					}

					_, back := generateAnkiCard(wordData)

					// Update the note
					err := updateNoteField(note.NoteID, backField, back)
					if err != nil {
						appendLog(fmt.Sprintf("❌ %s - 更新失败: %v", word, err))
						failed++
						continue
					}

					updated++
					appendLog(fmt.Sprintf("✅ %s → %s", word, wordData.Translation))

					// Small delay to not overwhelm LLM
					time.Sleep(100 * time.Millisecond)
				}

				progress := float64(end) / float64(len(noteIDs))
				progressBar.SetValue(progress)
				statusLabel.SetText(fmt.Sprintf("进度: %d/%d (更新: %d, 跳过: %d, 失败: %d)",
					end, len(noteIDs), updated, skipped, failed))
			}

			progressBar.SetValue(1)
			statusLabel.SetText(fmt.Sprintf("✅ 完成！更新: %d, 跳过: %d, 失败: %d", updated, skipped, failed))
			appendLog(fmt.Sprintf("\n完成！共更新 %d 张卡片", updated))
		}()
	}

	form := widget.NewForm(
		widget.NewFormItem("牌组", deckSelect),
	)

	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		startBtn,
		progressBar,
		statusLabel,
		logText,
	)

	win.SetContent(container.NewPadded(content))
	win.CenterOnScreen()
	win.Show()
}

// findNotesInDeck queries AnkiConnect for all note IDs in a deck
func findNotesInDeck(deckName string) ([]int64, error) {
	query := fmt.Sprintf(`"deck:%s"`, deckName)
	raw, err := queryAnkiConnect("findNotes", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	var ids []int64
	json.Unmarshal(raw, &ids)
	return ids, nil
}

type noteInfo struct {
	NoteID int64             `json:"noteId"`
	Fields map[string]string `json:"fields"`
	Tags   []string          `json:"tags"`
}

// fetchNotesInfo gets field values for a batch of note IDs
func fetchNotesInfo(noteIDs []int64) ([]noteInfo, error) {
	raw, err := queryAnkiConnect("notesInfo", map[string]any{"notes": noteIDs})
	if err != nil {
		return nil, err
	}
	var notes []noteInfo
	json.Unmarshal(raw, &notes)
	return notes, nil
}

// updateNoteField updates a single field of a note
func updateNoteField(noteID int64, fieldName, value string) error {
	_, err := queryAnkiConnect("updateNoteFields", map[string]any{
		"note": map[string]any{
			"id":     noteID,
			"fields": map[string]string{fieldName: value},
		},
	})
	return err
}
