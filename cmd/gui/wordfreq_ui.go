package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

// wordRow holds UI elements for a single word entry in the results list
type wordRow struct {
	entry    *WordFreqEntry
	check    *widget.Check
	countLbl *widget.Label
	srcLbl   *widget.Label
	row      fyne.CanvasObject
}

func showWordFreqDialog() {
	win := fyneApp.NewWindow("PDF 词频分析")
	win.Resize(fyne.NewSize(620, 760))

	store := loadWordFreqStore()

	// --- PDF list section ---
	pdfList := container.NewVBox()
	pdfScroll := container.NewVScroll(pdfList)
	pdfScroll.SetMinSize(fyne.NewSize(580, 80))

	// --- Filter section ---
	minFreqEntry := widget.NewEntry()
	minFreqEntry.SetText("2")
	minFreqEntry.SetPlaceHolder("2")

	maxWordsEntry := widget.NewEntry()
	maxWordsEntry.SetText("100")
	maxWordsEntry.SetPlaceHolder("100")

	excludeStopCheck := widget.NewCheck("排除停用词", nil)
	excludeStopCheck.SetChecked(true)

	excludeAnkiCheck := widget.NewCheck("排除已在 Anki 的词", nil)
	excludeAnkiCheck.SetChecked(true)

	// --- Deck selector ---
	deckSelect := widget.NewSelect([]string{}, nil)
	deckSelect.PlaceHolder = "选择牌组..."
	// Try to load deck names on open
	if isAnkiConnectAvailable() {
		if decks, err := fetchDeckNames(); err == nil && len(decks) > 0 {
			deckSelect.Options = decks
			deckSelect.SetSelected(ankiConfig.DeckName)
		}
	}

	// --- Results section ---
	var wordRows []*wordRow
	resultsBox := container.NewVBox()
	resultsScroll := container.NewVScroll(resultsBox)
	resultsScroll.SetMinSize(fyne.NewSize(580, 250))

	// Header for results
	resultsHeader := container.NewBorder(nil, nil,
		widget.NewLabel("✓"),
		container.NewHBox(
			widget.NewLabel("次数"),
			widget.NewLabel("    "),
			widget.NewLabel("来源"),
		),
		widget.NewLabel("单词"),
	)

	// --- Progress section ---
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	logText := widget.NewMultiLineEntry()
	logText.Wrapping = fyne.TextWrapWord
	logText.Disable()
	logText.SetMinRowsVisible(6)

	appendLog := func(msg string) {
		logText.SetText(logText.Text + msg + "\n")
	}

	// --- Buttons ---
	addPdfBtn := widget.NewButton("添加 PDF", nil)
	clearAllBtn := widget.NewButton("清除全部", nil)
	selectAllBtn := widget.NewButton("全选", nil)
	deselectAllBtn := widget.NewButton("取消全选", nil)
	importBtn := widget.NewButton("导入 Anki", nil)
	importBtn.Importance = widget.HighImportance

	var running bool

	// Forward-declare rebuild functions to allow mutual recursion
	var rebuildPDFList func()
	var rebuildResults func()

	// rebuildPDFList refreshes the PDF file list UI
	rebuildPDFList = func() {
		pdfList.Objects = nil
		for _, path := range store.Files {
			p := path // capture
			name := store.FileNames[p]
			if name == "" {
				name = filepath.Base(p)
			}
			row := container.NewBorder(nil, nil, nil,
				widget.NewButton("删除", func() {
					removePDF(p, store)
					saveWordFreqStore(store)
					rebuildPDFList()
					rebuildResults()
				}),
				widget.NewLabel(name),
			)
			pdfList.Add(row)
		}
		pdfList.Refresh()
		pdfScroll.Refresh()
	}

	// rebuildResults refreshes the word list based on current filters
	rebuildResults = func() {
		wordRows = nil
		resultsBox.Objects = nil

		minFreq := 2
		if v, err := strconv.Atoi(minFreqEntry.Text); err == nil && v > 0 {
			minFreq = v
		}

		maxWords := 100
		if v, err := strconv.Atoi(maxWordsEntry.Text); err == nil && v > 0 {
			maxWords = v
		}

		// Get words already in Anki if filter is enabled
		var ankiWords map[string]bool
		if excludeAnkiCheck.Checked {
			if words, err := getWordsAlreadyInAnki(); err == nil {
				ankiWords = words
			}
		}

		shown := 0
		for i := range store.Words {
			entry := &store.Words[i]

			if entry.Count < minFreq {
				break // sorted by count desc, so we can stop early
			}

			if excludeAnkiCheck.Checked && ankiWords != nil && ankiWords[entry.Word] {
				continue
			}

			if shown >= maxWords {
				break
			}
			shown++

			check := widget.NewCheck("", nil)
			if !entry.Imported {
				check.SetChecked(true)
			}

			wordLbl := widget.NewLabel(entry.Word)
			countLbl := widget.NewLabel(fmt.Sprintf("%d", entry.Count))
			srcLbl := widget.NewLabel(strings.Join(entry.Sources, ", "))

			row := container.NewBorder(nil, nil,
				check,
				container.NewHBox(countLbl, widget.NewLabel("  "), srcLbl),
				wordLbl,
			)

			wordRows = append(wordRows, &wordRow{
				entry:    entry,
				check:    check,
				countLbl: countLbl,
				srcLbl:   srcLbl,
				row:      row,
			})
			resultsBox.Add(row)
		}

		resultsBox.Refresh()
		resultsScroll.Refresh()

		totalWords := len(store.Words)
		statusLabel.SetText(fmt.Sprintf("共 %d 个单词, 显示 %d 个", totalWords, shown))
	}

	// --- Wire up buttons ---

	addPdfBtn.OnTapped = func() {
		if running {
			return
		}
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			path := reader.URI().Path()
			reader.Close()

			statusLabel.SetText("正在解析 PDF...")
			go func() {
				name := filepath.Base(path)
				result, err := processPDF(path, store, func(page, total int) {
					statusLabel.SetText(fmt.Sprintf("OCR 识别中... 第 %d/%d 页", page, total))
				})
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("解析失败: %v", err))
					return
				}
				saveWordFreqStore(store)
				rebuildPDFList()
				rebuildResults()
				method := "文本提取"
				if result.UsedOCR {
					method = "OCR 识别"
				}
				statusLabel.SetText(fmt.Sprintf("已添加: %s (%s %d 字符, %d 个不同单词)",
					name, method, result.TextLength, result.UniqueWords))
			}()
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".pdf"}))
		fd.Show()
	}

	clearAllBtn.OnTapped = func() {
		if running {
			return
		}
		dialog.ShowConfirm("确认", "确定清除所有 PDF 和词频数据？", func(ok bool) {
			if !ok {
				return
			}
			store.Files = nil
			store.FileNames = make(map[string]string)
			store.Words = nil
			saveWordFreqStore(store)
			rebuildPDFList()
			rebuildResults()
		}, win)
	}

	selectAllBtn.OnTapped = func() {
		for _, r := range wordRows {
			if !r.entry.Imported {
				r.check.SetChecked(true)
			}
		}
	}

	deselectAllBtn.OnTapped = func() {
		for _, r := range wordRows {
			r.check.SetChecked(false)
		}
	}

	importBtn.OnTapped = func() {
		if running {
			return
		}
		if !isAnkiConnectAvailable() {
			dialog.ShowInformation("Anki 未连接", "请先启动 Anki", win)
			return
		}

		// Refresh deck list
		decks, err := fetchDeckNames()
		if err != nil || len(decks) == 0 {
			dialog.ShowError(fmt.Errorf("获取牌组列表失败"), win)
			return
		}
		deckSelect.Options = decks
		if deckSelect.Selected == "" {
			deckSelect.SetSelected(ankiConfig.DeckName)
		}

		selectedDeck := deckSelect.Selected
		if selectedDeck == "" {
			selectedDeck = ankiConfig.DeckName
		}

		// Collect checked words
		var selected []string
		for _, r := range wordRows {
			if r.check.Checked && !r.entry.Imported {
				selected = append(selected, r.entry.Word)
			}
		}
		if len(selected) == 0 {
			dialog.ShowInformation("提示", "请至少选择一个单词", win)
			return
		}

		running = true
		addPdfBtn.Disable()
		clearAllBtn.Disable()
		selectAllBtn.Disable()
		deselectAllBtn.Disable()
		importBtn.Disable()
		progressBar.Show()
		progressBar.SetValue(0)
		logText.SetText("")

		targetDeck := selectedDeck
		cb := &translateCallback{
			OnLog: appendLog,
			OnProgress: func(current, total int64) {
				progressBar.SetValue(float64(current) / float64(total))
			},
			OnStatus: func(msg string) {
				statusLabel.SetText(msg)
			},
		}

		go func() {
			defer func() {
				running = false
				addPdfBtn.Enable()
				clearAllBtn.Enable()
				selectAllBtn.Enable()
				deselectAllBtn.Enable()
				importBtn.Enable()
			}()

			startTime := time.Now()
			statusLabel.SetText(fmt.Sprintf("正在导入 %d 个单词到 [%s]...", len(selected), targetDeck))

			success, failed := importWordsToAnki(selected, targetDeck, cb)
			elapsed := time.Since(startTime).Round(time.Millisecond)

			progressBar.SetValue(1)
			statusLabel.SetText(fmt.Sprintf("完成！成功: %d, 失败: %d, 耗时: %s", success, failed, elapsed))
			appendLog(fmt.Sprintf("\n导入完成！成功: %d, 失败: %d, 耗时: %s", success, failed, elapsed))

			// Mark imported words
			importedSet := make(map[string]bool)
			for _, w := range selected {
				importedSet[w] = true
			}
			for i := range store.Words {
				if importedSet[store.Words[i].Word] {
					store.Words[i].Imported = true
				}
			}
			saveWordFreqStore(store)

			// Refresh results to reflect imported status
			rebuildResults()
		}()
	}

	// Rebuild results when filters change
	minFreqEntry.OnChanged = func(s string) { rebuildResults() }
	maxWordsEntry.OnChanged = func(s string) { rebuildResults() }
	excludeAnkiCheck.OnChanged = func(b bool) { rebuildResults() }

	// --- Layout ---
	content := container.NewVBox(
		container.NewHBox(addPdfBtn, clearAllBtn),
		widget.NewLabel("PDF 文件:"),
		pdfScroll,
		widget.NewSeparator(),
		widget.NewLabel("筛选条件:"),
		container.NewGridWithColumns(2,
			container.NewHBox(widget.NewLabel("最低频率:"), minFreqEntry),
			container.NewHBox(widget.NewLabel("最大显示数:"), maxWordsEntry),
		),
		container.NewHBox(excludeStopCheck, excludeAnkiCheck),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, nil,
			resultsHeader,
		),
		resultsScroll,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			container.NewHBox(widget.NewLabel("导入到牌组:"), deckSelect),
			container.NewHBox(selectAllBtn, deselectAllBtn, importBtn),
		),
		widget.NewSeparator(),
		progressBar,
		statusLabel,
		logText,
	)

	win.SetContent(container.NewPadded(content))
	win.CenterOnScreen()

	// Initial load
	rebuildPDFList()
	if len(store.Files) > 0 {
		rebuildResults()
	}

	win.Show()
}
