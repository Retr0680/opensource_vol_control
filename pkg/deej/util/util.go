package util

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"go.uber.org/zap"
)

// EnsureDirExists creates the given directory path if it doesn't already exist.
func EnsureDirExists(path string) error {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("ensure directory exists (%s): %w", path, err)
	}
	return nil
}

// FileExists checks if a file exists and is not a directory.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	return !os.IsNotExist(err) && !info.IsDir()
}

// Linux returns true if we're running on Linux.
func Linux() bool {
	return runtime.GOOS == "linux"
}

// SetupCloseHandler creates a listener on a new goroutine that will notify
// the program if it receives an interrupt signal from the OS.
func SetupCloseHandler() chan os.Signal {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c
}

// GetCurrentWindowProcessNames returns the process names of the current foreground window,
// including child processes. Currently only implemented for Windows.
func GetCurrentWindowProcessNames() ([]string, error) {
	return getCurrentWindowProcessNames()
}

// OpenExternal spawns a detached process (e.g., opening a file or URL) with the given command and argument.
func OpenExternal(logger *zap.SugaredLogger, cmd string, arg string) error {
	command := createExternalCommand(cmd, arg)
	if err := command.Run(); err != nil {
		logger.Warnw("Failed to spawn detached process", "command", cmd, "argument", arg, "error", err)
		return fmt.Errorf("spawn detached proc: %w", err)
	}
	return nil
}

// NormalizeScalar trims the given float32 to 2 decimal places of precision (e.g., 0.15442 -> 0.15).
// Used for normalizing audio volume levels and slider values.
func NormalizeScalar(v float32) float32 {
	return float32(math.Floor(float64(v)*100) / 100.0)
}

// SignificantlyDifferent returns true if there's a significant enough volume difference between two values,
// considering a specified noise reduction level.
func SignificantlyDifferent(old float32, new float32, noiseReductionLevel string) bool {
	threshold := getSignificantDifferenceThreshold(noiseReductionLevel)
	if math.Abs(float64(old-new)) >= threshold {
		return true
	}
	// Special behavior around edges of 0.0 and 1.0.
	if (almostEquals(new, 1.0) && old != 1.0) || (almostEquals(new, 0.0) && old != 0.0) {
		return true
	}
	return false
}

// almostEquals checks if two float32 values are very close to each other.
func almostEquals(a float32, b float32) bool {
	return math.Abs(float64(a-b)) < 0.000001
}

// createExternalCommand prepares the appropriate command for launching an external process depending on the OS.
func createExternalCommand(cmd string, arg string) *exec.Cmd {
	if Linux() {
		// Use bash for Linux.
		return exec.Command("/bin/bash", "-c", fmt.Sprintf("%s %s", cmd, arg))
	}
	// Default to cmd.exe for Windows.
	return exec.Command("cmd.exe", "/C", "start", "/b", cmd, arg)
}

// getSignificantDifferenceThreshold returns the threshold for considering a volume difference significant,
// based on the provided noise reduction level.
func getSignificantDifferenceThreshold(noiseReductionLevel string) float64 {
	const (
		noiseReductionHigh = "high"
		noiseReductionLow  = "low"
	)
	switch noiseReductionLevel {
	case noiseReductionHigh:
		return 0.035
	case noiseReductionLow:
		return 0.015
	default:
		return 0.025
	}
}