package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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

// noteTask represents a single note to be translated
type noteTask struct {
	note noteInfo
	word string
}

// translateCallback provides progress updates to the UI
type translateCallback struct {
	OnLog      func(msg string)
	OnProgress func(current, total int64)
	OnStatus   func(msg string)
}

// retranslateDeck translates all notes in a deck, returns (updated, skipped, failed)
func retranslateDeck(deckName string, cb *translateCallback) (int64, int, int64) {
	frontField := ankiConfig.FrontField
	backField := ankiConfig.BackField
	if frontField == "" || backField == "" {
		cb.OnStatus("❌ 请先在设置中配置 Anki 字段")
		return 0, 0, 0
	}

	noteIDs, err := findNotesInDeck(deckName)
	if err != nil {
		cb.OnLog(fmt.Sprintf("⚠️ %s - 查询失败: %v", deckName, err))
		return 0, 0, 0
	}
	if len(noteIDs) == 0 {
		cb.OnLog(fmt.Sprintf("📭 %s - 空牌组", deckName))
		return 0, 0, 0
	}

	allNotes, err := fetchNotesInfo(noteIDs)
	if err != nil {
		cb.OnLog(fmt.Sprintf("⚠️ %s - 获取笔记失败: %v", deckName, err))
		return 0, 0, 0
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

	if len(tasks) == 0 {
		return 0, skippedCount, 0
	}

	cb.OnLog(fmt.Sprintf("📦 %s: %d 个笔记", deckName, len(tasks)))

	var updated, failed, done int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	taskCh := make(chan noteTask, len(tasks))

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
					cb.OnLog(fmt.Sprintf("⚠️ %s - 翻译失败", t.word))
					mu.Unlock()
					continue
				}

				front, back := generateAnkiCard(wordData)

				// Update both front (phonetic + 谐音) and back (translation + examples etc.)
				if err := updateNoteFields(t.note.NoteID, frontField, front, backField, back); err != nil {
					atomic.AddInt64(&failed, 1)
					atomic.AddInt64(&done, 1)
					mu.Lock()
					cb.OnLog(fmt.Sprintf("❌ %s - 更新失败: %v", t.word, err))
					mu.Unlock()
					continue
				}

				atomic.AddInt64(&updated, 1)
				cur := atomic.AddInt64(&done, 1)
				mu.Lock()
				cb.OnLog(fmt.Sprintf("✅ %s → %s", t.word, wordData.Translation))
				cb.OnProgress(cur, int64(len(tasks)))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return updated, skippedCount, failed
}

// showRetranslateDialog shows the re-translate window with single/all deck options
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
	win.Resize(fyne.NewSize(520, 450))

	deckSelect := widget.NewSelect(decks, func(s string) { selectedDeck = s })
	deckSelect.SetSelected(selectedDeck)

	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	logText := widget.NewMultiLineEntry()
	logText.Wrapping = fyne.TextWrapWord
	logText.Disable()
	logText.SetMinRowsVisible(8)

	var running bool

	appendLog := func(msg string) {
		logText.SetText(logText.Text + msg + "\n")
		logText.CursorRow = len(strings.Split(logText.Text, "\n")) - 1
	}

	makeCb := func() *translateCallback {
		return &translateCallback{
			OnLog: appendLog,
			OnProgress: func(current, total int64) {
				progressBar.SetValue(float64(current) / float64(total))
			},
			OnStatus: func(msg string) {
				statusLabel.SetText(msg)
			},
		}
	}

	// --- Buttons ---
	singleBtn := widget.NewButton("▶ 刷新选中牌组", nil)
	singleBtn.Importance = widget.HighImportance

	allBtn := widget.NewButton("⚡ 一键翻译所有", nil)
	allBtn.Importance = widget.WarningImportance

	disableAll := func() {
		running = true
		singleBtn.Disable()
		allBtn.Disable()
		progressBar.Show()
		progressBar.SetValue(0)
		logText.SetText("")
	}
	enableAll := func() {
		running = false
		singleBtn.Enable()
		allBtn.Enable()
	}

	singleBtn.OnTapped = func() {
		if running {
			return
		}
		disableAll()
		statusLabel.SetText(fmt.Sprintf("正在翻译 %s ...", selectedDeck))

		go func() {
			defer enableAll()
			startTime := time.Now()
			cb := makeCb()
			updated, skipped, failed := retranslateDeck(selectedDeck, cb)
			elapsed := time.Since(startTime).Round(time.Millisecond)
			progressBar.SetValue(1)
			statusLabel.SetText(fmt.Sprintf("✅ %s 完成！更新: %d, 跳过: %d, 失败: %d, 耗时: %s",
				selectedDeck, updated, skipped, failed, elapsed))
			appendLog(fmt.Sprintf("\n🎉 完成！共更新 %d 张卡片, 耗时 %s", updated, elapsed))
		}()
	}

	allBtn.OnTapped = func() {
		if running {
			return
		}
		disableAll()
		statusLabel.SetText("正在翻译所有牌组...")

		go func() {
			defer enableAll()
			startTime := time.Now()
			cb := makeCb()

			var totalUpdated, totalFailed int64
			totalSkipped := 0

			for i, deck := range decks {
				appendLog(fmt.Sprintf("\n━━━ [%d/%d] %s ━━━", i+1, len(decks), deck))
				statusLabel.SetText(fmt.Sprintf("📦 [%d/%d] %s", i+1, len(decks), deck))
				progressBar.SetValue(float64(i) / float64(len(decks)))

				updated, skipped, failed := retranslateDeck(deck, cb)
				totalUpdated += updated
				totalSkipped += skipped
				totalFailed += failed
			}

			elapsed := time.Since(startTime).Round(time.Millisecond)
			progressBar.SetValue(1)
			statusLabel.SetText(fmt.Sprintf("✅ 全部完成！%d 个牌组, 更新: %d, 跳过: %d, 失败: %d, 耗时: %s",
				len(decks), totalUpdated, totalSkipped, totalFailed, elapsed))
			appendLog(fmt.Sprintf("\n🎉 全部完成！共 %d 个牌组, 更新 %d 张卡片, 耗时 %s",
				len(decks), totalUpdated, elapsed))
		}()
	}

	// --- Layout ---
	deckForm := widget.NewForm(
		widget.NewFormItem("牌组", deckSelect),
	)
	buttons := container.NewGridWithColumns(2, singleBtn, allBtn)

	content := container.NewVBox(
		deckForm,
		widget.NewSeparator(),
		buttons,
		widget.NewSeparator(),
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

// updateNoteFields updates both front and back fields of a note in one call
func updateNoteFields(noteID int64, frontField, frontValue, backField, backValue string) error {
	_, err := queryAnkiConnect("updateNoteFields", map[string]any{
		"note": map[string]any{
			"id": noteID,
			"fields": map[string]string{
				frontField: frontValue,
				backField:  backValue,
			},
		},
	})
	return err
}
