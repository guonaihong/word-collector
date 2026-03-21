package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Carbon -framework Cocoa

#include <Carbon/Carbon.h>
#include <dispatch/dispatch.h>

// Hotkey IDs
enum {
    kHotkeyCollect = 1,  // ⌃⌥⌘W
    kHotkeyToggle  = 2,  // ⌃⌥⌘S
};

// Go callback declarations
extern void goHotkeyCollect();
extern void goHotkeyToggle();

// Carbon hotkey handler
static OSStatus hotkeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    EventHotKeyID hotkeyID;
    GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof(hotkeyID), NULL, &hotkeyID);

    switch (hotkeyID.id) {
        case kHotkeyCollect:
            goHotkeyCollect();
            break;
        case kHotkeyToggle:
            goHotkeyToggle();
            break;
    }
    return noErr;
}

static EventHotKeyRef gCollectHotKey = NULL;
static EventHotKeyRef gToggleHotKey = NULL;

// Register global hotkeys using Carbon API
static int registerHotkeys() {
    EventTypeSpec eventType;
    eventType.eventClass = kEventClassKeyboard;
    eventType.eventKind = kEventHotKeyPressed;

    InstallApplicationEventHandler(&hotkeyHandler, 1, &eventType, NULL, NULL);

    EventHotKeyID collectID = {.signature = 'WCol', .id = kHotkeyCollect};
    EventHotKeyID toggleID  = {.signature = 'WCol', .id = kHotkeyToggle};

    // ⌃⌥⌘W: kVK_ANSI_W = 13, controlKey | optionKey | cmdKey
    OSStatus s1 = RegisterEventHotKey(13, controlKey | optionKey | cmdKey, collectID,
        GetApplicationEventTarget(), 0, &gCollectHotKey);

    // ⌃⌥⌘S: kVK_ANSI_S = 1, controlKey | optionKey | cmdKey
    OSStatus s2 = RegisterEventHotKey(1, controlKey | optionKey | cmdKey, toggleID,
        GetApplicationEventTarget(), 0, &gToggleHotKey);

    if (s1 != noErr || s2 != noErr) {
        return -1;
    }
    return 0;
}

static void unregisterHotkeys() {
    if (gCollectHotKey) {
        UnregisterEventHotKey(gCollectHotKey);
        gCollectHotKey = NULL;
    }
    if (gToggleHotKey) {
        UnregisterEventHotKey(gToggleHotKey);
        gToggleHotKey = NULL;
    }
}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

//export goHotkeyCollect
func goHotkeyCollect() {
	if !appState.IsEnabled {
		return
	}

	// Copy selected text via Cmd+C simulation using AppleScript
	script := `
	tell application "System Events"
		keystroke "c" using {command down}
	end tell`
	exec.Command("osascript", "-e", script).Run()

	// Wait for clipboard to update
	time.Sleep(200 * time.Millisecond)

	// Get clipboard content
	word := getClipboard()
	word = strings.TrimSpace(word)
	if word == "" {
		return
	}

	// Check word count
	if len(strings.Fields(word)) > 3 {
		return
	}

	// Add the word (runs on main goroutine via channel to avoid UI thread issues)
	go func() {
		addWord(word)
	}()
}

//export goHotkeyToggle
func goHotkeyToggle() {
	go func() {
		toggleEnabled()
	}()
}

func setupGlobalHotkeys() error {
	result := C.registerHotkeys()
	if result != 0 {
		return fmt.Errorf("failed to register global hotkeys (need accessibility permission?)")
	}
	fmt.Println("✅ Global hotkeys registered: ⌃⌥⌘W (collect), ⌃⌥⌘S (toggle)")
	return nil
}

func cleanupGlobalHotkeys() {
	C.unregisterHotkeys()
}
