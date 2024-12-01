package deej

import (
	"strings"

	"go.uber.org/zap"
)

// Session represents a single addressable audio session
type Session interface {
	// GetVolume returns the current volume of the session.
	GetVolume() float32

	// SetVolume adjusts the session's volume to the specified value.
	SetVolume(v float32) error

	// TODO: future mute support
	// GetMute() bool
	// SetMute(m bool) error

	// Key returns a unique identifier for the session.
	Key() string

	// Release releases any resources associated with the session.
	Release()
}

const (
	// sessionCreationLogMessage is logged when a new audio session is created.
	sessionCreationLogMessage = "Created audio session instance"

	// sessionStringFormat is the format used when displaying session details.
	// It includes the human-readable description and current volume.
	sessionStringFormat = "<session: %s, vol: %.2f>"
)

type baseSession struct {
	logger *zap.SugaredLogger
	system bool
	master bool

	// Name should be set by the child to uniquely identify the session.
	// Can be the process name or session type (e.g., system or master).
	name string

	// Human-readable description to be used when displaying the session.
	// For example: "Chrome (pid 1234)" or "System Sounds".
	humanReadableDesc string
}

// Key generates a unique identifier for the session based on its type.
func (s *baseSession) Key() string {
	if s.system {
		return systemSessionName // The system session uses a predefined constant
	}

	// Return the session name in lowercase for consistency.
	// Master sessions and others will have unique names, e.g., "mic" or device name.
	return strings.ToLower(s.name)
}

// Release is a placeholder in the base session for child classes to implement their cleanup logic.
func (s *baseSession) Release() {
	// Base session might not require specific cleanup, but this ensures that child sessions
	// can override and add their cleanup logic.
	s.logger.Debug("Releasing base session")
}