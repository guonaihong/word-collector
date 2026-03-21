package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showDeckSetupDialog shows a settings window with Anki and Translation tabs
func showDeckSetupDialog() {
	mainWindow.Show()
	mainWindow.RequestFocus()

	settingsWindow := fyneApp.NewWindow("⚙ 设置")
	settingsWindow.Resize(fyne.NewSize(480, 400))

	ankiTab := buildAnkiTab(settingsWindow)
	translateTab := buildTranslateTab(settingsWindow)

	tabs := container.NewAppTabs(
		container.NewTabItem("Anki", ankiTab),
		container.NewTabItem("翻译", translateTab),
	)

	settingsWindow.SetContent(container.NewPadded(tabs))
	settingsWindow.CenterOnScreen()
	settingsWindow.Show()
}

// buildAnkiTab creates the Anki configuration tab
func buildAnkiTab(win fyne.Window) fyne.CanvasObject {
	if !isAnkiConnectAvailable() {
		return container.NewCenter(
			widget.NewLabel("请先启动 Anki，AnkiConnect 插件已自动安装。\n重启 Anki 后再配置。"),
		)
	}

	decks, err := fetchDeckNames()
	if err != nil || len(decks) == 0 {
		return container.NewCenter(widget.NewLabel("获取牌组失败，请确认 Anki 已启动"))
	}

	models, err := fetchModelNames()
	if err != nil || len(models) == 0 {
		return container.NewCenter(widget.NewLabel("获取模板失败，请确认 Anki 已启动"))
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

	selectedModel := models[0]
	if ankiConfig.ModelName != "" {
		for _, m := range models {
			if m == ankiConfig.ModelName {
				selectedModel = m
				break
			}
		}
	}

	deckSelect := widget.NewSelect(decks, func(s string) { selectedDeck = s })
	deckSelect.SetSelected(selectedDeck)

	modelSelect := widget.NewSelect(models, nil)
	frontFieldSelect := widget.NewSelect([]string{}, nil)
	backFieldSelect := widget.NewSelect([]string{}, nil)

	var selectedFront, selectedBack string

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

		if len(fields) >= 1 {
			selectedFront = fields[0]
			frontFieldSelect.SetSelected(fields[0])
		}
		if len(fields) >= 2 {
			selectedBack = fields[1]
			backFieldSelect.SetSelected(fields[1])
		}

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

	form := widget.NewForm(
		widget.NewFormItem("牌组", deckSelect),
		widget.NewFormItem("模板", modelSelect),
		widget.NewFormItem("正面字段", frontFieldSelect),
		widget.NewFormItem("背面字段", backFieldSelect),
	)

	saveBtn := widget.NewButton("保存", func() {
		if selectedDeck == "" || selectedModel == "" || selectedFront == "" || selectedBack == "" {
			dialog.ShowError(fmt.Errorf("请选择所有选项"), win)
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
	})
	saveBtn.Importance = widget.HighImportance

	return container.NewVBox(form, widget.NewSeparator(), saveBtn)
}

// buildTranslateTab creates the Translation/LLM configuration tab
func buildTranslateTab(win fyne.Window) fyne.CanvasObject {
	sourceOptions := []string{"有道词典", "LLM (Kimi/DeepSeek/...)"}
	sourceSelect := widget.NewSelect(sourceOptions, nil)

	// Map display name to config value
	sourceMap := map[string]string{
		"有道词典":                    "youdao",
		"LLM (Kimi/DeepSeek/...)": "llm",
	}
	reverseMap := map[string]string{
		"youdao": "有道词典",
		"llm":    "LLM (Kimi/DeepSeek/...)",
	}

	// Restore current config
	currentSource := ankiConfig.TranslateSource
	if currentSource == "" {
		currentSource = "youdao"
	}
	if display, ok := reverseMap[currentSource]; ok {
		sourceSelect.SetSelected(display)
	}

	endpointEntry := widget.NewEntry()
	endpointEntry.SetPlaceHolder("https://api.moonshot.cn/v1")
	endpointEntry.SetText(ankiConfig.LLMEndpoint)

	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetPlaceHolder("sk-...")
	apiKeyEntry.SetText(ankiConfig.LLMAPIKey)

	modelEntry := widget.NewEntry()
	modelEntry.SetPlaceHolder("moonshot-v1-8k")
	modelEntry.SetText(ankiConfig.LLMModel)

	// Preset buttons for common providers
	presets := container.NewGridWithColumns(4,
		widget.NewButton("LM Studio", func() {
			endpointEntry.SetText("http://localhost:1234/v1")
			modelEntry.SetText("qwen/qwen3-30b-a3b-2507")
			apiKeyEntry.SetText("lm-studio")
		}),
		widget.NewButton("Kimi", func() {
			endpointEntry.SetText("https://api.moonshot.cn/v1")
			modelEntry.SetText("moonshot-v1-8k")
		}),
		widget.NewButton("DeepSeek", func() {
			endpointEntry.SetText("https://api.deepseek.com/v1")
			modelEntry.SetText("deepseek-chat")
		}),
		widget.NewButton("OpenAI", func() {
			endpointEntry.SetText("https://api.openai.com/v1")
			modelEntry.SetText("gpt-4o-mini")
		}),
	)

	llmForm := widget.NewForm(
		widget.NewFormItem("API 地址", endpointEntry),
		widget.NewFormItem("API Key", apiKeyEntry),
		widget.NewFormItem("模型", modelEntry),
	)

	llmSection := container.NewVBox(
		widget.NewLabelWithStyle("快速填充:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		presets,
		widget.NewSeparator(),
		llmForm,
	)

	// Show/hide LLM section based on source
	llmSection.Hide()
	if currentSource == "llm" {
		llmSection.Show()
	}

	sourceSelect.OnChanged = func(s string) {
		if sourceMap[s] == "llm" {
			llmSection.Show()
		} else {
			llmSection.Hide()
		}
	}

	mainForm := widget.NewForm(
		widget.NewFormItem("翻译源", sourceSelect),
	)

	saveBtn := widget.NewButton("保存", func() {
		selected := sourceSelect.Selected
		ankiConfig.TranslateSource = sourceMap[selected]

		if sourceMap[selected] == "llm" {
			if endpointEntry.Text == "" || apiKeyEntry.Text == "" || modelEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("请填写完整的 LLM 配置"), win)
				return
			}
			ankiConfig.LLMEndpoint = endpointEntry.Text
			ankiConfig.LLMAPIKey = apiKeyEntry.Text
			ankiConfig.LLMModel = modelEntry.Text
		}

		saveAnkiConfig()
		fmt.Printf("✅ Translation config saved: source=%s\n", ankiConfig.TranslateSource)
		showNotification("Word Collector", "翻译配置已保存: "+selected)
	})
	saveBtn.Importance = widget.HighImportance

	return container.NewVBox(mainForm, llmSection, widget.NewSeparator(), saveBtn)
}
