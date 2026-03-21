package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework Carbon -framework Cocoa

#include <ApplicationServices/ApplicationServices.h>
#include <Carbon/Carbon.h>

// Go callback declarations
extern void goHotkeyCollect();
extern void goHotkeyToggle();

// Required modifier flags for matching
// kCGEventFlagMaskControl | kCGEventFlagMaskAlternate | kCGEventFlagMaskCommand
static const CGEventFlags kRequiredMods = kCGEventFlagMaskControl | kCGEventFlagMaskAlternate | kCGEventFlagMaskCommand;
// Mask to isolate modifier keys (ignore caps lock, fn, etc.)
static const CGEventFlags kModMask = kCGEventFlagMaskControl | kCGEventFlagMaskAlternate | kCGEventFlagMaskCommand | kCGEventFlagMaskShift;

static CFMachPortRef gEventTap = NULL;
static CFRunLoopSourceRef gRunLoopSource = NULL;
static CFRunLoopRef gRunLoop = NULL;

static CGEventRef hotkeyCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo) {
    // Re-enable tap if it gets disabled by the system (e.g. timeout)
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        CGEventTapEnable(gEventTap, true);
        return event;
    }

    if (type != kCGEventKeyDown) {
        return event;
    }

    CGEventFlags flags = CGEventGetFlags(event);
    CGKeyCode keyCode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);

    // Check if exactly ⌃⌥⌘ are held (no shift)
    if ((flags & kModMask) != kRequiredMods) {
        return event;
    }

    // kVK_ANSI_W = 13
    if (keyCode == 13) {
        goHotkeyCollect();
        return NULL; // consume the event
    }

    // kVK_ANSI_S = 1
    if (keyCode == 1) {
        goHotkeyToggle();
        return NULL; // consume the event
    }

    return event;
}

static int startEventTap() {
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown);

    gEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,  // active tap, can consume events
        mask,
        hotkeyCallback,
        NULL
    );

    if (!gEventTap) {
        return -1; // likely no accessibility permission
    }

    gRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, gEventTap, 0);
    gRunLoop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(gRunLoop, gRunLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(gEventTap, true);

    // Run this thread's run loop (blocks until stopped)
    CFRunLoopRun();
    return 0;
}

static void stopEventTap() {
    if (gEventTap) {
        CGEventTapEnable(gEventTap, false);
    }
    if (gRunLoop) {
        CFRunLoopStop(gRunLoop);
    }
    if (gRunLoopSource) {
        CFRelease(gRunLoopSource);
        gRunLoopSource = NULL;
    }
    if (gEventTap) {
        CFRelease(gEventTap);
        gEventTap = NULL;
    }
}

// Dummy callback for accessibility test
static CGEventRef dummyCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *userInfo) {
    return event;
}

// Test if we have accessibility permission by trying to create a tap
static int testAccessibility() {
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown);
    CFMachPortRef tap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        mask,
        dummyCallback,
        NULL
    );
    if (!tap) {
        return -1;
    }
    CFRelease(tap);
    return 0;
}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

//export goHotkeyCollect
func goHotkeyCollect() {
	if !appState.IsEnabled {
		return
	}

	go func() {
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
	// Test accessibility permission first
	if C.testAccessibility() != 0 {
		return fmt.Errorf("no accessibility permission — grant it in System Settings → Privacy & Security → Accessibility")
	}

	// Start the real event tap on a dedicated OS thread with its own RunLoop
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		result := C.startEventTap()
		if result != 0 {
			fmt.Println("⚠️  Failed to start event tap")
		}
	}()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	fmt.Println("✅ Global hotkeys registered: ⌃⌥⌘W (collect), ⌃⌥⌘S (toggle)")
	return nil
}

func cleanupGlobalHotkeys() {
	C.stopEventTap()
}
