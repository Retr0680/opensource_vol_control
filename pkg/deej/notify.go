package deej

import (
	"os"
	"path/filepath"

	"github.com/gen2brain/beeep"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/icon"
	"github.com/omriharel/deej/pkg/deej/util"
)

// Notifier provides a generic interface for sending notifications.
type Notifier interface {
	Notify(title string, message string)
}

// ToastNotifier handles sending toast notifications on Windows systems.
type ToastNotifier struct {
	logger *zap.SugaredLogger
}

// NewToastNotifier creates a new instance of ToastNotifier.
func NewToastNotifier(logger *zap.SugaredLogger) (*ToastNotifier, error) {
	logger = logger.Named("notifier")
	logger.Debug("Created toast notifier instance")

	return &ToastNotifier{logger: logger}, nil
}

// Notify sends a toast notification. If the notification icon is missing, it creates it dynamically.
func (tn *ToastNotifier) Notify(title, message string) {
	appIconPath := filepath.Join(os.TempDir(), "deej.ico")

	// Ensure the icon file exists.
	if err := tn.ensureIconFile(appIconPath); err != nil {
		tn.logger.Errorw("Failed to prepare toast notification icon", "error", err)
		return
	}

	tn.logger.Infow("Sending toast notification", "title", title, "message", message)

	// Send the notification.
	if err := beeep.Notify(title, message, appIconPath); err != nil {
		tn.logger.Errorw("Failed to send toast notification", "error", err)
	}
}

// ensureIconFile checks if the icon file exists, and creates it if necessary.
func (tn *ToastNotifier) ensureIconFile(path string) error {
	if util.FileExists(path) {
		return nil
	}

	tn.logger.Debugw("Deej icon file missing, creating", "path", path)

	// Create the icon file and write the content.
	if err := os.WriteFile(path, icon.DeejLogo, 0644); err != nil {
		return err
	}

	tn.logger.Debugw("Successfully created toast notification icon", "path", path)
	return nil
}