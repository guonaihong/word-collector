package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showDeckSetupDialog shows a dialog for the user to select Anki deck, model and fields
func showDeckSetupDialog() {
	// Ensure Anki is running and AnkiConnect is available
	if !isAnkiConnectAvailable() {
		dialog.ShowInformation("Anki 未连接",
			"请先启动 Anki，然后点击 ⚙ 设置按钮配置牌组。\n\n"+
				"AnkiConnect 插件已自动安装，\n重启 Anki 即可激活。",
			mainWindow)
		mainWindow.Show()
		mainWindow.RequestFocus()
		return
	}

	mainWindow.Show()
	mainWindow.RequestFocus()

	// Fetch deck names
	decks, err := fetchDeckNames()
	if err != nil {
		dialog.ShowError(fmt.Errorf("获取牌组失败: %v", err), mainWindow)
		return
	}
	if len(decks) == 0 {
		dialog.ShowError(fmt.Errorf("Anki 中没有牌组"), mainWindow)
		return
	}

	// Fetch model names
	models, err := fetchModelNames()
	if err != nil {
		dialog.ShowError(fmt.Errorf("获取模板失败: %v", err), mainWindow)
		return
	}
	if len(models) == 0 {
		dialog.ShowError(fmt.Errorf("Anki 中没有模板"), mainWindow)
		return
	}

	// Set defaults from current config or first available
	selectedDeck := decks[0]
	if ankiConfig.DeckName != "" {
		for _, d := range decks {
			if d == ankiConfig.DeckName {
				selectedDeck = d
				break
			}
		}
	}

	selectedModel := models[0]
	if ankiConfig.ModelName != "" {
		for _, m := range models {
			if m == ankiConfig.ModelName {
				selectedModel = m
				break
			}
		}
	}

	// UI elements
	deckSelect := widget.NewSelect(decks, func(s string) {
		selectedDeck = s
	})
	deckSelect.SetSelected(selectedDeck)

	modelSelect := widget.NewSelect(models, nil)
	frontFieldSelect := widget.NewSelect([]string{}, nil)
	backFieldSelect := widget.NewSelect([]string{}, nil)

	var selectedFront, selectedBack string

	// When model changes, update field selects
	updateFields := func(modelName string) {
		fields, err := fetchModelFieldNames(modelName)
		if err != nil || len(fields) == 0 {
			frontFieldSelect.Options = []string{}
			backFieldSelect.Options = []string{}
			frontFieldSelect.Refresh()
			backFieldSelect.Refresh()
			return
		}
		frontFieldSelect.Options = fields
		backFieldSelect.Options = fields

		// Auto-select: first field = front, second field = back
		if len(fields) >= 1 {
			selectedFront = fields[0]
			frontFieldSelect.SetSelected(fields[0])
		}
		if len(fields) >= 2 {
			selectedBack = fields[1]
			backFieldSelect.SetSelected(fields[1])
		}

		// Restore from config if matching
		if ankiConfig.FrontField != "" {
			for _, f := range fields {
				if f == ankiConfig.FrontField {
					selectedFront = f
					frontFieldSelect.SetSelected(f)
					break
				}
			}
		}
		if ankiConfig.BackField != "" {
			for _, f := range fields {
				if f == ankiConfig.BackField {
					selectedBack = f
					backFieldSelect.SetSelected(f)
					break
				}
			}
		}
	}

	frontFieldSelect.OnChanged = func(s string) { selectedFront = s }
	backFieldSelect.OnChanged = func(s string) { selectedBack = s }

	modelSelect.OnChanged = func(s string) {
		selectedModel = s
		updateFields(s)
	}
	modelSelect.SetSelected(selectedModel)

	// Build form
	form := container.NewVBox(
		widget.NewLabelWithStyle("Anki 设置", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("牌组 (Deck):"),
		deckSelect,
		widget.NewLabel("模板 (Note Type):"),
		modelSelect,
		widget.NewLabel("正面字段 (Front):"),
		frontFieldSelect,
		widget.NewLabel("背面字段 (Back):"),
		backFieldSelect,
	)

	d := dialog.NewCustomConfirm("⚙ 配置 Anki", "保存", "取消", form, func(ok bool) {
		if !ok {
			return
		}
		if selectedDeck == "" || selectedModel == "" || selectedFront == "" || selectedBack == "" {
			dialog.ShowError(fmt.Errorf("请选择所有选项"), mainWindow)
			return
		}
		ankiConfig.DeckName = selectedDeck
		ankiConfig.ModelName = selectedModel
		ankiConfig.FrontField = selectedFront
		ankiConfig.BackField = selectedBack
		saveAnkiConfig()
		fmt.Printf("✅ Anki config saved: deck=%s, model=%s, front=%s, back=%s\n",
			selectedDeck, selectedModel, selectedFront, selectedBack)
		showNotification("Word Collector", "Anki 配置已保存: "+selectedDeck)
	}, mainWindow)
	d.Resize(fyne.NewSize(350, 400))
	d.Show()
}
