package deej

import (
	"github.com/getlantern/systray"
	"github.com/omriharel/deej/pkg/deej/icon"
	"github.com/omriharel/deej/pkg/deej/util"
)

const (
	editConfigTitle       = "Edit configuration"
	editConfigTooltip     = "Open config file with notepad"
	refreshSessionsTitle  = "Re-scan audio sessions"
	refreshSessionsTooltip = "Manually refresh audio sessions if something's stuck"
	quitTitle             = "Quit"
	quitTooltip           = "Stop deej and quit"
)

func (d *Deej) initializeTray(onDone func()) {
	logger := d.logger.Named("tray")

	// Define the onReady callback to handle systray actions
	onReady := func() {
		logger.Debug("Tray instance ready")

		// Set tray icon, title, and tooltip
		systray.SetTemplateIcon(icon.DeejLogo, icon.DeejLogo)
		systray.SetTitle("deej")
		systray.SetTooltip("deej")

		// Create menu items
		editConfig := systray.AddMenuItem(editConfigTitle, editConfigTooltip)
		editConfig.SetIcon(icon.EditConfig)

		refreshSessions := systray.AddMenuItem(refreshSessionsTitle, refreshSessionsTooltip)
		refreshSessions.SetIcon(icon.RefreshSessions)

		if d.version != "" {
			systray.AddSeparator()
			versionInfo := systray.AddMenuItem(d.version, "")
			versionInfo.Disable()
		}

		systray.AddSeparator()
		quit := systray.AddMenuItem(quitTitle, quitTooltip)

		// Wait for actions in a separate goroutine
		go d.handleTrayActions(logger, editConfig, refreshSessions, quit)

		// Notify that tray setup is complete
		onDone()
	}

	// Define the onExit callback for when the tray is exited
	onExit := func() {
		logger.Debug("Tray exited")
	}

	// Start the tray
	logger.Debug("Running in tray")
	systray.Run(onReady, onExit)
}

func (d *Deej) handleTrayActions(logger *zap.SugaredLogger, editConfig, refreshSessions, quit *systray.MenuItem) {
	for {
		select {
		// Quit the application
		case <-quit.ClickedCh:
			logger.Info("Quit menu item clicked, stopping")
			d.signalStop()

		// Open the configuration file for editing
		case <-editConfig.ClickedCh:
			logger.Info("Edit config menu item clicked, opening config for editing")
			editor := getEditor()

			if err := util.OpenExternal(logger, editor, userConfigFilepath); err != nil {
				logger.Warnw("Failed to open config file for editing", "error", err)
			}

		// Refresh the audio sessions
		case <-refreshSessions.ClickedCh:
			logger.Info("Refresh sessions menu item clicked, triggering session map refresh")
			d.sessions.refreshSessions(true)
		}
	}
}

func getEditor() string {
	// Determine the appropriate editor based on the operating system
	if util.Linux() {
		return "gedit"
	}
	// Default to notepad.exe for Windows and other OS
	return "notepad.exe"
}

func (d *Deej) stopTray() {
	d.logger.Debug("Quitting tray")
	systray.Quit()
}