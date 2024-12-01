// Package deej provides a machine-side client that pairs with an Arduino
// chip to form a tactile, physical volume control system.
package deej

import (
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

const (
	// EnvNoTray disables the tray icon when set.
	EnvNoTray = "DEEJ_NO_TRAY_ICON"
)

// Deej manages the main application components.
type Deej struct {
	logger      *zap.SugaredLogger
	notifier    Notifier
	config      *CanonicalConfig
	serial      *SerialIO
	sessions    *sessionMap
	stopChannel chan bool
	version     string
	verbose     bool
}

// NewDeej creates a new Deej instance.
func NewDeej(logger *zap.SugaredLogger, verbose bool) (*Deej, error) {
	logger = logger.Named("deej")

	notifier, err := NewToastNotifier(logger)
	if err != nil {
		logger.Errorw("Failed to create notifier", "error", err)
		return nil, fmt.Errorf("failed to create notifier: %w", err)
	}

	config, err := NewConfig(logger, notifier)
	if err != nil {
		logger.Errorw("Failed to create configuration", "error", err)
		return nil, fmt.Errorf("failed to create configuration: %w", err)
	}

	serial, err := NewSerialIO(nil, logger)
	if err != nil {
		logger.Errorw("Failed to initialize serial communication", "error", err)
		return nil, fmt.Errorf("failed to initialize serial communication: %w", err)
	}

	sessionFinder, err := newSessionFinder(logger)
	if err != nil {
		logger.Errorw("Failed to initialize session finder", "error", err)
		return nil, fmt.Errorf("failed to initialize session finder: %w", err)
	}

	sessions, err := newSessionMap(nil, logger, sessionFinder)
	if err != nil {
		logger.Errorw("Failed to initialize session map", "error", err)
		return nil, fmt.Errorf("failed to initialize session map: %w", err)
	}

	d := &Deej{
		logger:      logger,
		notifier:    notifier,
		config:      config,
		serial:      serial,
		sessions:    sessions,
		stopChannel: make(chan bool),
		verbose:     verbose,
	}

	serial.SetParent(d)
	sessions.SetParent(d)

	logger.Debug("Deej instance created successfully")
	return d, nil
}

// Initialize prepares components and starts running the application.
func (d *Deej) Initialize() error {
	d.logger.Debug("Initializing deej")

	if err := d.config.Load(); err != nil {
		d.logger.Errorw("Failed to load configuration", "error", err)
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := d.sessions.initialize(); err != nil {
		d.logger.Errorw("Failed to initialize session map", "error", err)
		return fmt.Errorf("failed to initialize session map: %w", err)
	}

	if os.Getenv(EnvNoTray) != "" {
		d.logger.Debug("Running without tray icon")
		d.setupInterruptHandler()
		d.run()
	} else {
		d.setupInterruptHandler()
		d.initializeTray(d.run)
	}

	return nil
}

// SetVersion sets the application version for display in the tray menu.
func (d *Deej) SetVersion(version string) {
	d.version = version
}

// Verbose indicates whether the application runs in verbose mode.
func (d *Deej) Verbose() bool {
	return d.verbose
}

func (d *Deej) setupInterruptHandler() {
	interruptChannel := util.SetupCloseHandler()

	go func() {
		signal := <-interruptChannel
		d.logger.Debugw("Interrupt received", "signal", signal)
		d.signalStop()
	}()
}

func (d *Deej) run() {
	d.logger.Info("Run loop starting")

	go d.config.WatchConfigFileChanges()

	go func() {
		if err := d.serial.Start(); err != nil {
			d.handleSerialError(err)
		}
	}()

	<-d.stopChannel
	d.logger.Debug("Stop signal received")

	if err := d.stop(); err != nil {
		d.logger.Warnw("Error during shutdown", "error", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func (d *Deej) handleSerialError(err error) {
	switch {
	case errors.Is(err, os.ErrPermission):
		d.logger.Warnw("Serial port busy", "comPort", d.config.ConnectionInfo.COMPort)
		d.notifier.Notify("Serial port busy!",
			"Close other applications using the port and try again.")
	case errors.Is(err, os.ErrNotExist):
		d.logger.Warnw("Invalid serial port configuration", "comPort", d.config.ConnectionInfo.COMPort)
		d.notifier.Notify("Invalid serial port!",
			"Ensure the correct port is set in the configuration.")
	default:
		d.logger.Warnw("Unknown error during serial start", "error", err)
	}
	d.signalStop()
}

func (d *Deej) signalStop() {
	d.logger.Debug("Sending stop signal")
	d.stopChannel <- true
}

func (d *Deej) stop() error {
	d.logger.Info("Shutting down deej")

	d.config.StopWatchingConfigFile()
	d.serial.Stop()

	if err := d.sessions.release(); err != nil {
		d.logger.Errorw("Failed to release session map", "error", err)
		return fmt.Errorf("failed to release session map: %w", err)
	}

	d.stopTray()
	d.logger.Sync()
	return nil
}