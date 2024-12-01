package deej

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/omriharel/deej/pkg/deej/util"
)

const (
	crashlogFilename        = "deej-crash-%s.log"
	crashlogTimestampFormat = "2006.01.02-15.04.05"
	crashMessageTemplate    = `-----------------------------------------------------------------
                        deej crashlog
-----------------------------------------------------------------
Unfortunately, deej has crashed. This really shouldn't happen!
If you've just encountered this, please contact @omriharel and attach this error log.
You can also join the deej Discord server at https://discord.gg/nf88NJu.
-----------------------------------------------------------------
Time: %s
Panic occurred: %s
Stack trace:
%s
-----------------------------------------------------------------
`
)

// recoverFromPanic handles application panics, logs the error, and attempts to shut down gracefully.
func (d *Deej) recoverFromPanic() {
	if r := recover(); r != nil {
		d.handlePanic(r)
	}
}

// handlePanic logs the panic details, writes a crash log file, and notifies the user.
func (d *Deej) handlePanic(recoverValue interface{}) {
	now := time.Now()
	crashlogPath := filepath.Join(logDirectory, fmt.Sprintf(crashlogFilename, now.Format(crashlogTimestampFormat)))

	// Create the crash log content.
	crashLogContent := d.createCrashLogContent(now, recoverValue)

	// Ensure the log directory exists.
	if err := util.EnsureDirExists(logDirectory); err != nil {
		panic(fmt.Errorf("failed to create log directory: %w", err))
	}

	// Write the crash log file.
	if err := os.WriteFile(crashlogPath, crashLogContent, 0644); err != nil {
		panic(fmt.Errorf("failed to write crash log: %w", err))
	}

	// Log and notify the crash.
	d.logger.Errorw("Application panic encountered",
		"crashlogPath", crashlogPath,
		"error", recoverValue)

	d.notifier.Notify("Unexpected crash occurred",
		fmt.Sprintf("Details logged to: %s", crashlogPath))

	// Attempt to shut down gracefully.
	d.signalStop()

	// Exit with an error code.
	d.logger.Errorw("Exiting due to panic", "exitCode", 1)
	os.Exit(1)
}

// createCrashLogContent generates the formatted crash log content.
func (d *Deej) createCrashLogContent(timestamp time.Time, recoverValue interface{}) []byte {
	return []byte(fmt.Sprintf(crashMessageTemplate,
		timestamp.Format(crashlogTimestampFormat),
		recoverValue,
		debug.Stack(),
	))
}