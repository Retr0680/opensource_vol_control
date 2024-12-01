package deej

// Session represents an audio session with specific details, including playback state.
type Session interface {
	// Play starts the session (audio playback)
	Play() error
	
	// Pause pauses the session
	Pause() error
	
	// Stop stops the session
	Stop() error
	
	// GetName returns the name of the session (e.g., application name)
	GetName() string
}

// SessionFinder defines methods for discovering and managing audio sessions.
type SessionFinder interface {
	// GetAllSessions returns a list of all active audio sessions. It might return stale data if the device has been changed recently.
	// Returns an error if the discovery process fails.
	GetAllSessions() ([]Session, error)

	// Release frees any resources allocated by the SessionFinder. It is important to call Release once done using the SessionFinder.
	Release() error
}