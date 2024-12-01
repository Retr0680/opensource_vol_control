package util

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"github.com/mitchellh/go-ps"
)

const (
	// Cooldown duration to avoid frequent calls to GetCurrentWindowProcessNames.
	getCurrentWindowInternalCooldown = time.Millisecond * 350
)

var (
	// Cache the result and the last call timestamp to avoid frequent API calls.
	lastGetCurrentWindowResult []string
	lastGetCurrentWindowCall   = time.Now()
)

// getCurrentWindowProcessNames retrieves the process names of the currently focused window and its child windows
// (if applicable), considering UWP apps and processes running in container apps (e.g., Steam, League Client).
func getCurrentWindowProcessNames() ([]string, error) {
	// Apply an internal cooldown to avoid excessive API calls.
	now := time.Now()
	if lastGetCurrentWindowCall.Add(getCurrentWindowInternalCooldown).After(now) {
		// Return cached results during cooldown period
		return lastGetCurrentWindowResult, nil
	}

	lastGetCurrentWindowCall = now

	// Initialize the result slice to store process names
	var result []string

	// Callback function for enumerating child windows of the foreground window.
	enumChildWindowsCallback := func(childHWND *uintptr, lParam *uintptr) uintptr {
		// Cast lParam to get the owner PID (parent process PID)
		ownerPID := (*uint32)(unsafe.Pointer(lParam))

		// Get the child window's real PID
		var childPID uint32
		win.GetWindowThreadProcessId((win.HWND)(unsafe.Pointer(childHWND)), &childPID)

		// If child PID is different from owner PID, add the child's process name to the result list
		if childPID != *ownerPID {
			processName, err := getProcessNameByPID(childPID)
			if err != nil {
				return 1 // Continue enumerating child windows
			}
			result = append(result, processName)
		}

		return 1 // Continue enumerating child windows
	}

	// Get the current foreground window and its owner (parent) PID
	hwnd := win.GetForegroundWindow()
	var ownerPID uint32
	win.GetWindowThreadProcessId(hwnd, &ownerPID)

	// If the parent process PID is 0 (system PID), return an empty result
	if ownerPID == 0 {
		return nil, nil
	}

	// Find the process name of the parent window and add it to the result
	processName, err := getProcessNameByPID(ownerPID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent process for PID %d: %w", ownerPID, err)
	}
	result = append(result, processName)

	// Enumerate child windows and add their process names if they differ from the parent
	win.EnumChildWindows(hwnd, syscall.NewCallback(enumChildWindowsCallback), (uintptr)(unsafe.Pointer(&ownerPID)))

	// Cache the result for future use
	lastGetCurrentWindowResult = result
	return result, nil
}

// getProcessNameByPID retrieves the process name of the process corresponding to the provided PID.
func getProcessNameByPID(pid uint32) (string, error) {
	process, err := ps.FindProcess(int(pid))
	if err != nil {
		return "", fmt.Errorf("failed to find process for PID %d: %w", pid, err)
	}
	return process.Executable(), nil
}