package deej

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
	"go.uber.org/zap"
)

type wcaSessionFinder struct {
	logger        *zap.SugaredLogger
	sessionLogger *zap.SugaredLogger

	eventCtx *ole.GUID // Context for audio session notifications

	// Device change notifications
	mmDeviceEnumerator      *wca.IMMDeviceEnumerator
	mmNotificationClient    *wca.IMMNotificationClient
	lastDefaultDeviceChange time.Time

	// Master input and output sessions
	masterOut *masterSession
	masterIn  *masterSession
}

const (
	// Unique GUID for the event context
	mysteriousGUID = "{1ec920a1-7db8-44ba-9779-e5d28ed9f330}"

	// Threshold to filter out rapid notifications
	minDefaultDeviceChangeThreshold = 100 * time.Millisecond

	// Prefix for device session logs
	deviceSessionFormat = "device.%s"
)

func newSessionFinder(logger *zap.SugaredLogger) (SessionFinder, error) {
	sf := &wcaSessionFinder{
		logger:        logger.Named("session_finder"),
		sessionLogger: logger.Named("sessions"),
		eventCtx:      ole.NewGUID(mysteriousGUID),
	}

	sf.logger.Debug("Created WCA session finder instance")

	return sf, nil
}

func (sf *wcaSessionFinder) GetAllSessions() ([]Session, error) {
	sessions := []Session{}

	// Initialize COM
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		// Handle redundant initialization gracefully
		if oleErr, ok := err.(*ole.OleError); ok && oleErr.Code() == 1 {
			sf.logger.Warn("CoInitializeEx called redundantly")
		} else {
			sf.logger.Warnw("Failed to initialize COM library", "error", err)
			return nil, fmt.Errorf("initialize COM: %w", err)
		}
	}
	defer ole.CoUninitialize()

	// Ensure device enumerator is available
	if err := sf.getDeviceEnumerator(); err != nil {
		sf.logger.Warnw("Failed to get device enumerator", "error", err)
		return nil, fmt.Errorf("get device enumerator: %w", err)
	}

	// Get default audio endpoints
	defaultOutputEndpoint, defaultInputEndpoint, err := sf.getDefaultAudioEndpoints()
	if err != nil {
		sf.logger.Warnw("Failed to get default audio endpoints", "error", err)
		return nil, fmt.Errorf("get default audio endpoints: %w", err)
	}
	defer defaultOutputEndpoint.Release()

	if defaultInputEndpoint != nil {
		defer defaultInputEndpoint.Release()
	}

	// Register for device change notifications if not already registered
	if sf.mmNotificationClient == nil {
		if err := sf.registerDefaultDeviceChangeCallback(); err != nil {
			sf.logger.Warnw("Failed to register device change callback", "error", err)
			return nil, fmt.Errorf("register device change callback: %w", err)
		}
	}

	// Retrieve master output session
	sf.masterOut, err = sf.getMasterSession(defaultOutputEndpoint, masterSessionName, masterSessionName)
	if err != nil {
		sf.logger.Warnw("Failed to retrieve master audio output session", "error", err)
		return nil, fmt.Errorf("get master output session: %w", err)
	}
	sessions = append(sessions, sf.masterOut)

	// Retrieve master input session if available
	if defaultInputEndpoint != nil {
		sf.masterIn, err = sf.getMasterSession(defaultInputEndpoint, inputSessionName, inputSessionName)
		if err != nil {
			sf.logger.Warnw("Failed to retrieve master audio input session", "error", err)
			return nil, fmt.Errorf("get master input session: %w", err)
		}
		sessions = append(sessions, sf.masterIn)
	}

	// Enumerate device and process sessions
	if err := sf.enumerateAndAddSessions(&sessions); err != nil {
		sf.logger.Warnw("Failed to enumerate audio sessions", "error", err)
		return nil, fmt.Errorf("enumerate sessions: %w", err)
	}

	return sessions, nil
}

func (sf *wcaSessionFinder) Release() error {
	if sf.mmDeviceEnumerator != nil {
		sf.mmDeviceEnumerator.Release()
	}
	sf.logger.Debug("Released WCA session finder instance")
	return nil
}

func (sf *wcaSessionFinder) getDeviceEnumerator() error {
	if sf.mmDeviceEnumerator == nil {
		if err := wca.CoCreateInstance(
			wca.CLSID_MMDeviceEnumerator,
			0,
			wca.CLSCTX_ALL,
			wca.IID_IMMDeviceEnumerator,
			&sf.mmDeviceEnumerator,
		); err != nil {
			sf.logger.Warnw("Failed to create device enumerator", "error", err)
			return fmt.Errorf("create device enumerator: %w", err)
		}
	}
	return nil
}

func (sf *wcaSessionFinder) defaultDeviceChangedCallback(
	this *wca.IMMNotificationClient,
	EDataFlow, eRole uint32,
	lpcwstr uintptr,
) uintptr {
	now := time.Now()
	if now.Sub(sf.lastDefaultDeviceChange) < minDefaultDeviceChangeThreshold {
		return 0
	}
	sf.lastDefaultDeviceChange = now

	sf.logger.Debug("Default audio device changed. Marking master sessions as stale.")
	if sf.masterOut != nil {
		sf.masterOut.markAsStale()
	}
	if sf.masterIn != nil {
		sf.masterIn.markAsStale()
	}
	return 0
}