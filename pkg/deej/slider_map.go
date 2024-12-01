package deej

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/thoas/go-funk"
)

type sliderMap struct {
	m    map[int][]string
	lock sync.RWMutex // Use RWMutex for better performance on reads
}

func newSliderMap() *sliderMap {
	return &sliderMap{
		m:    make(map[int][]string),
		lock: sync.RWMutex{},
	}
}

// sliderMapFromConfigs initializes a new sliderMap from user and internal mappings.
func sliderMapFromConfigs(userMapping map[string][]string, internalMapping map[string][]string) *sliderMap {
	resultMap := newSliderMap()

	// Copy targets from user config, ignoring empty values
	for sliderIdxString, targets := range userMapping {
		sliderIdx, err := strconv.Atoi(sliderIdxString)
		if err != nil {
			// Log error or handle gracefully
			continue
		}

		resultMap.set(sliderIdx, funk.FilterString(targets, func(s string) bool {
			return s != ""
		}))
	}

	// Add targets from internal configs, ignoring duplicate or empty values
	for sliderIdxString, targets := range internalMapping {
		sliderIdx, err := strconv.Atoi(sliderIdxString)
		if err != nil {
			// Log error or handle gracefully
			continue
		}

		existingTargets, _ := resultMap.get(sliderIdx)
		filteredTargets := funk.FilterString(targets, func(s string) bool {
			return s != "" && !funk.ContainsString(existingTargets, s)
		})

		existingTargets = append(existingTargets, filteredTargets...)
		resultMap.set(sliderIdx, existingTargets)
	}

	return resultMap
}

// iterate runs the provided function on each slider in the map.
func (m *sliderMap) iterate(f func(int, []string)) {
	m.lock.RLock() // Use RLock for read-only access
	defer m.lock.RUnlock()

	for key, value := range m.m {
		f(key, value)
	}
}

// get retrieves the targets for the specified slider index.
func (m *sliderMap) get(key int) ([]string, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	value, ok := m.m[key]
	return value, ok
}

// set updates or adds the targets for the specified slider index.
func (m *sliderMap) set(key int, value []string) {
	m.lock.Lock() // Use Lock for write access
	defer m.lock.Unlock()

	m.m[key] = value
}

// String returns a human-readable representation of the sliderMap.
func (m *sliderMap) String() string {
	m.lock.RLock() // Use RLock for read-only access
	defer m.lock.RUnlock()

	sliderCount := len(m.m)
	targetCount := 0

	for _, targets := range m.m {
		targetCount += len(targets)
	}

	return fmt.Sprintf("<%d sliders mapped to %d targets>", sliderCount, targetCount)
}