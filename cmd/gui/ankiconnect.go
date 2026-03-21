package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ankiConnectAddonID = "2055492159"

// Minimal AnkiConnect plugin - supports addNote and version check
// This is a subset of the full AnkiConnect plugin (https://github.com/FooSoft/anki-connect)
const ankiConnectPlugin = `import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
import threading

from anki.notes import Note
from aqt import mw

PLUGIN_VERSION = "2.0"
print(f"[WordCollector AnkiConnect v{PLUGIN_VERSION}] loaded")

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(f"AnkiConnect v{PLUGIN_VERSION}".encode())

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        action = body.get("action", "")
        params = body.get("params", {})
        result, error = None, None
        try:
            if action == "version":
                result = 6
            elif action == "addNote":
                result = add_note(params)
            elif action == "deckNames":
                result = [d["name"] for d in mw.col.decks.all()]
            elif action == "modelNames":
                result = [m["name"] for m in mw.col.models.all()]
            elif action == "modelFieldNames":
                model_name = params.get("modelName", "")
                model = mw.col.models.by_name(model_name)
                if model:
                    result = [f["name"] for f in model["flds"]]
                else:
                    error = f"model not found: {model_name}"
            else:
                error = f"unsupported action: {action}"
        except Exception as e:
            error = str(e)
        resp = json.dumps({"result": result, "error": error})
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(resp.encode())

    def log_message(self, format, *args):
        pass

def add_note(params):
    note_params = params.get("note", {})
    deck_name = note_params.get("deckName", "Default")
    model_name = note_params.get("modelName", "Basic")
    fields = note_params.get("fields", {})
    tags = note_params.get("tags", [])

    col = mw.col
    model = col.models.by_name(model_name)
    if not model:
        raise Exception(f"model not found: {model_name}")

    deck = col.decks.by_name(deck_name)
    if not deck:
        did = col.decks.id(deck_name)
    else:
        did = deck["id"]

    note = Note(col, model)
    for name, value in fields.items():
        if name in note:
            note[name] = value
    for tag in tags:
        note.tags.append(tag)

    # Duplicate check: search for notes with same first field
    opts = note_params.get("options", {})
    if not opts.get("allowDuplicate", False):
        first_field_name = model["flds"][0]["name"] if model["flds"] else None
        if first_field_name and first_field_name in fields:
            first_val = fields[first_field_name].strip()
            if first_val:
                # Strip HTML tags for comparison
                import re
                clean = re.sub(r'<[^>]+>', '', first_val).strip()
                if clean:
                    existing = col.find_notes(f'"{clean}"')
                    if existing:
                        raise Exception("duplicate: note with same content already exists")

    # Monkey-patch dupes if missing (Anki 25.x removed it)
    if not hasattr(note, 'dupes'):
        note.dupes = lambda: []

    # Try multiple API versions
    try:
        col.add_note(note, did)
    except (AttributeError, TypeError):
        try:
            note.model()["did"] = did
            col.addNote(note)
        except Exception:
            raise Exception("failed to add note - unsupported Anki version")
    col.save()
    return note.id

def start_server():
    try:
        server = HTTPServer(("127.0.0.1", 8765), Handler)
        server.serve_forever()
    except Exception:
        pass

threading.Thread(target=start_server, daemon=True).start()
`

// getAnkiAddonsDir returns the Anki addons directory path
func getAnkiAddonsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Anki2", "addons21")
}

// isAnkiConnectInstalled checks if AnkiConnect addon is installed
func isAnkiConnectInstalled() bool {
	addonDir := filepath.Join(getAnkiAddonsDir(), ankiConnectAddonID)
	initFile := filepath.Join(addonDir, "__init__.py")
	_, err := os.Stat(initFile)
	return err == nil
}

// installAnkiConnect installs a minimal AnkiConnect plugin to Anki's addons directory
func installAnkiConnect() error {
	addonsDir := getAnkiAddonsDir()

	// Check if Anki2 directory exists
	anki2Dir := filepath.Dir(addonsDir)
	if _, err := os.Stat(anki2Dir); os.IsNotExist(err) {
		return fmt.Errorf("Anki not installed (directory not found: %s)", anki2Dir)
	}

	addonDir := filepath.Join(addonsDir, ankiConnectAddonID)
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		return fmt.Errorf("create addon directory: %w", err)
	}

	// Clear pycache to force Anki to reload the updated plugin
	os.RemoveAll(filepath.Join(addonDir, "__pycache__"))

	initFile := filepath.Join(addonDir, "__init__.py")
	if err := os.WriteFile(initFile, []byte(ankiConnectPlugin), 0644); err != nil {
		return fmt.Errorf("write plugin: %w", err)
	}

	// Create manifest
	manifest := `{"package": "2055492159", "name": "AnkiConnect (auto-installed by Word Collector)"}`
	manifestFile := filepath.Join(addonDir, "manifest.json")
	if err := os.WriteFile(manifestFile, []byte(manifest), 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// ensureAnkiConnect installs or updates AnkiConnect plugin, returns status message
func ensureAnkiConnect() string {
	wasInstalled := isAnkiConnectInstalled()

	if err := installAnkiConnect(); err != nil {
		return fmt.Sprintf("AnkiConnect install failed: %v", err)
	}

	if !wasInstalled {
		return "AnkiConnect plugin installed! Please restart Anki to activate."
	}
	return ""
}

// isAnkiRunning checks if Anki is currently running
func isAnkiRunning() bool {
	output, _ := execCommand("osascript", "-e",
		`tell application "System Events" to (name of processes) contains "Anki"`)
	return strings.TrimSpace(output) == "true"
}

func execCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}
