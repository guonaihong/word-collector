package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

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

			// Fetch all notes info first
			allNotes, err := fetchNotesInfo(noteIDs)
			if err != nil {
				statusLabel.SetText(fmt.Sprintf("❌ 获取笔记失败: %v", err))
				return
			}

			// Filter valid notes
			type noteTask struct {
				note noteInfo
				word string
			}
			var tasks []noteTask
			skippedCount := 0
			for _, note := range allNotes {
				frontVal, ok := note.Fields[frontField]
				if !ok || frontVal == "" {
					skippedCount++
					continue
				}
				word := stripHTML(frontVal)
				if word == "" {
					skippedCount++
					continue
				}
				tasks = append(tasks, noteTask{note: note, word: word})
			}

			total := len(tasks)
			appendLog(fmt.Sprintf("有效笔记: %d, 跳过: %d", total, skippedCount))

			// Concurrent processing with 4 workers
			var updated, failed, done int64
			var mu sync.Mutex
			var wg sync.WaitGroup
			taskCh := make(chan noteTask, total)

			for _, t := range tasks {
				taskCh <- t
			}
			close(taskCh)

			workers := 4
			wg.Add(workers)
			for w := 0; w < workers; w++ {
				go func() {
					defer wg.Done()
					for t := range taskCh {
						wordData := fetchTranslation(t.word)
						if wordData.Translation == "" || wordData.Translation == "[Add translation]" {
							atomic.AddInt64(&failed, 1)
							atomic.AddInt64(&done, 1)
							mu.Lock()
							appendLog(fmt.Sprintf("⚠️ %s - 翻译失败", t.word))
							mu.Unlock()
							continue
						}

						_, back := generateAnkiCard(wordData)

						if err := updateNoteField(t.note.NoteID, backField, back); err != nil {
							atomic.AddInt64(&failed, 1)
							atomic.AddInt64(&done, 1)
							mu.Lock()
							appendLog(fmt.Sprintf("❌ %s - 更新失败: %v", t.word, err))
							mu.Unlock()
							continue
						}

						atomic.AddInt64(&updated, 1)
						cur := atomic.AddInt64(&done, 1)
						mu.Lock()
						appendLog(fmt.Sprintf("✅ %s → %s", t.word, wordData.Translation))
						progressBar.SetValue(float64(cur) / float64(total))
						statusLabel.SetText(fmt.Sprintf("进度: %d/%d (更新: %d, 失败: %d)",
							cur, total, atomic.LoadInt64(&updated), atomic.LoadInt64(&failed)))
						mu.Unlock()
					}
				}()
			}
			wg.Wait()

			progressBar.SetValue(1)
			statusLabel.SetText(fmt.Sprintf("✅ 完成！更新: %d, 跳过: %d, 失败: %d", updated, skippedCount, failed))
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
