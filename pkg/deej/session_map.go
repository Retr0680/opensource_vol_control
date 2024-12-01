package deej

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/omriharel/deej/pkg/deej/util"
	"github.com/thoas/go-funk"
	"go.uber.org/zap"
)

const (
	masterSessionName           = "master"           // master device volume
	systemSessionName           = "system"           // system sounds volume
	inputSessionName            = "mic"              // microphone input level
	specialTargetTransformPrefix = "deej."
	specialTargetCurrentWindow  = "current"
	specialTargetAllUnmapped   = "unmapped"
	minTimeBetweenSessionRefreshes = time.Second * 5
	maxTimeBetweenSessionRefreshes = time.Second * 45
)

// this matches friendly device names (on Windows), e.g. "Headphones (Realtek Audio)"
var deviceSessionKeyPattern = regexp.MustCompile(`^.+ \(.+\)$`)

type sessionMap struct {
	deej              *Deej
	logger            *zap.SugaredLogger
	m                 map[string][]Session
	lock              sync.Locker
	sessionFinder     SessionFinder
	lastSessionRefresh time.Time
	unmappedSessions  []Session
}

func newSessionMap(deej *Deej, logger *zap.SugaredLogger, sessionFinder SessionFinder) (*sessionMap, error) {
	logger = logger.Named("sessions")

	m := &sessionMap{
		deej:          deej,
		logger:        logger,
		m:             make(map[string][]Session),
		lock:          &sync.Mutex{},
		sessionFinder: sessionFinder,
	}

	logger.Debug("Created session map instance")

	return m, nil
}

func (m *sessionMap) initialize() error {
	if err := m.getAndAddSessions(); err != nil {
		m.logger.Warnw("Failed to get all sessions during session map initialization", "error", err)
		return fmt.Errorf("get all sessions during init: %w", err)
	}

	m.setupOnConfigReload()
	m.setupOnSliderMove()

	return nil
}

func (m *sessionMap) release() error {
	if err := m.sessionFinder.Release(); err != nil {
		m.logger.Warnw("Failed to release session finder during session map release", "error", err)
		return fmt.Errorf("release session finder during release: %w", err)
	}

	return nil
}

// assumes the session map is clean!
// only call on a new session map or as part of refreshSessions which calls reset
func (m *sessionMap) getAndAddSessions() error {
	// mark that we're refreshing before anything else
	m.lastSessionRefresh = time.Now()
	m.unmappedSessions = nil

	sessions, err := m.sessionFinder.GetAllSessions()
	if err != nil {
		m.logger.Warnw("Failed to get sessions from session finder", "error", err)
		return fmt.Errorf("get sessions from SessionFinder: %w", err)
	}

	for _, session := range sessions {
		m.add(session)

		if !m.sessionMapped(session) {
			m.logger.Debugw("Tracking unmapped session", "session", session)
			m.unmappedSessions = append(m.unmappedSessions, session)
		}
	}

	m.logger.Infow("Got all audio sessions successfully", "sessionMap", m)

	return nil
}

func (m *sessionMap) setupOnConfigReload() {
	configReloadedChannel := m.deej.config.SubscribeToChanges()

	go func() {
		for {
			select {
			case <-configReloadedChannel:
				m.logger.Info("Detected config reload, attempting to re-acquire all audio sessions")
				m.refreshSessions(false)
			}
		}
	}()
}

func (m *sessionMap) setupOnSliderMove() {
	sliderEventsChannel := m.deej.serial.SubscribeToSliderMoveEvents()

	go func() {
		for {
			select {
			case event := <-sliderEventsChannel:
				m.handleSliderMoveEvent(event)
			}
		}
	}()
}

// refreshes sessions with a forced refresh flag
func (m *sessionMap) refreshSessions(force bool) {
	if !force && m.lastSessionRefresh.Add(minTimeBetweenSessionRefreshes).After(time.Now()) {
		return
	}

	m.clear()

	if err := m.getAndAddSessions(); err != nil {
		m.logger.Warnw("Failed to re-acquire all audio sessions", "error", err)
	} else {
		m.logger.Debug("Re-acquired sessions successfully")
	}
}

// returns true if a session is not currently mapped to any slider
func (m *sessionMap) sessionMapped(session Session) bool {
	// count master/system/mic as mapped
	if funk.ContainsString([]string{masterSessionName, systemSessionName, inputSessionName}, session.Key()) {
		return true
	}

	// count device sessions as mapped
	if deviceSessionKeyPattern.MatchString(session.Key()) {
		return true
	}

	matchFound := false
	m.deej.config.SliderMapping.iterate(func(sliderIdx int, targets []string) {
		for _, target := range targets {
			if m.targetHasSpecialTransform(target) {
				continue
			}

			// resolve the target and compare it
			resolvedTarget := m.resolveTarget(target)[0]
			if resolvedTarget == session.Key() {
				matchFound = true
				return
			}
		}
	})

	return matchFound
}

// handles the slider move events and updates volumes accordingly
func (m *sessionMap) handleSliderMoveEvent(event SliderMoveEvent) {
	if m.lastSessionRefresh.Add(maxTimeBetweenSessionRefreshes).Before(time.Now()) {
		m.logger.Debug("Stale session map detected on slider move, refreshing")
		m.refreshSessions(true)
	}

	targets, ok := m.deej.config.SliderMapping.get(event.SliderID)
	if !ok {
		return
	}

	targetFound := false
	adjustmentFailed := false

	for _, target := range targets {
		resolvedTargets := m.resolveTarget(target)

		for _, resolvedTarget := range resolvedTargets {
			sessions, ok := m.get(resolvedTarget)
			if !ok {
				continue
			}

			targetFound = true

			for _, session := range sessions {
				if session.GetVolume() != event.PercentValue {
					if err := session.SetVolume(event.PercentValue); err != nil {
						m.logger.Warnw("Failed to set target session volume", "error", err)
						adjustmentFailed = true
					}
				}
			}
		}
	}

	if !targetFound {
		m.refreshSessions(false)
	} else if adjustmentFailed {
		m.refreshSessions(true)
	}
}

func (m *sessionMap) targetHasSpecialTransform(target string) bool {
	return strings.HasPrefix(target, specialTargetTransformPrefix)
}

func (m *sessionMap) resolveTarget(target string) []string {
	target = strings.ToLower(target)

	if m.targetHasSpecialTransform(target) {
		return m.applyTargetTransform(strings.TrimPrefix(target, specialTargetTransformPrefix))
	}

	return []string{target}
}

func (m *sessionMap) applyTargetTransform(specialTargetName string) []string {
	switch specialTargetName {
	case specialTargetCurrentWindow:
		return m.getCurrentWindowProcessNames()
	case specialTargetAllUnmapped:
		return m.getUnmappedSessionKeys()
	}

	return nil
}

func (m *sessionMap) getCurrentWindowProcessNames() []string {
	currentWindowProcessNames, err := util.GetCurrentWindowProcessNames()
	if err != nil {
		m.logger.Warnw("Failed to get current window process names", "error", err)
		return nil
	}

	for i := range currentWindowProcessNames {
		currentWindowProcessNames[i] = strings.ToLower(currentWindowProcessNames[i])
	}

	return funk.UniqString(currentWindowProcessNames)
}

func (m *sessionMap) getUnmappedSessionKeys() []string {
	targetKeys := make([]string, len(m.unmappedSessions))
	for i, session := range m.unmappedSessions {
		targetKeys[i] = session.Key()
	}

	return targetKeys
}

func (m *sessionMap) add(value Session) {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := value.Key()

	if _, ok := m.m[key]; !ok {
		m.m[key] = []Session{value}
	} else {
		m.m[key] = append(m.m[key], value)
	}
}

func (m *sessionMap) get(key string) ([]Session, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	value, ok := m.m[key]
	return value, ok
}

func (m *sessionMap) clear() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.logger.Debug("Releasing and clearing all audio sessions")

	for key, sessions := range m.m {
		for _, session := range sessions {
			session.Release()
		}
		delete(m.m, key)
	}

	m.logger.Debug("Session map cleared")
}

func (m *sessionMap) String() string {
	m.lock.Lock()
	defer m.lock.Unlock()

	sessionCount := 0
	for _, sessions := range m.m {
		sessionCount += len(sessions)
	}

	return fmt.Sprintf("<%d audio sessions>", sessionCount)
}