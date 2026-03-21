package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// AnkiConfig stores the user's Anki deck/model preferences
type AnkiConfig struct {
	DeckName   string `json:"deck_name"`
	ModelName  string `json:"model_name"`
	FrontField string `json:"front_field"`
	BackField  string `json:"back_field"`
}

var ankiConfig *AnkiConfig

func getConfigFile() string {
	return expandPath("~/.wordcollector_config.json")
}

func loadAnkiConfig() {
	ankiConfig = &AnkiConfig{}
	if data, err := os.ReadFile(getConfigFile()); err == nil {
		json.Unmarshal(data, ankiConfig)
	}
}

func saveAnkiConfig() {
	data, _ := json.MarshalIndent(ankiConfig, "", "  ")
	dir := filepath.Dir(getConfigFile())
	os.MkdirAll(dir, 0755)
	os.WriteFile(getConfigFile(), data, 0644)
}

func isAnkiConfigured() bool {
	return ankiConfig != nil && ankiConfig.DeckName != "" && ankiConfig.ModelName != ""
}

// queryAnkiConnect sends a request to AnkiConnect and returns the result
func queryAnkiConnect(action string, params map[string]any) (json.RawMessage, error) {
	reqBody := map[string]any{
		"action":  action,
		"version": 6,
	}
	if params != nil {
		reqBody["params"] = params
	}
	jsonBody, _ := json.Marshal(reqBody)
	resp, err := http.Post("http://localhost:8765", "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *string         `json:"error"`
	}
	json.Unmarshal(body, &result)
	if result.Error != nil && *result.Error != "" {
		return nil, fmt.Errorf("%s", *result.Error)
	}
	return result.Result, nil
}

// fetchDeckNames gets all deck names from Anki
func fetchDeckNames() ([]string, error) {
	raw, err := queryAnkiConnect("deckNames", nil)
	if err != nil {
		return nil, err
	}
	var names []string
	json.Unmarshal(raw, &names)
	return names, nil
}

// fetchModelNames gets all model/note type names from Anki
func fetchModelNames() ([]string, error) {
	raw, err := queryAnkiConnect("modelNames", nil)
	if err != nil {
		return nil, err
	}
	var names []string
	json.Unmarshal(raw, &names)
	return names, nil
}

// fetchModelFieldNames gets field names for a given model
func fetchModelFieldNames(modelName string) ([]string, error) {
	raw, err := queryAnkiConnect("modelFieldNames", map[string]any{"modelName": modelName})
	if err != nil {
		return nil, err
	}
	var names []string
	json.Unmarshal(raw, &names)
	return names, nil
}
