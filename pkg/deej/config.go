package deej

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

// CanonicalConfig provides centralized access to configuration fields
type CanonicalConfig struct {
	SliderMapping       *sliderMap
	ConnectionInfo      ConnectionInfo
	InvertSliders       bool
	NoiseReductionLevel string

	logger             *zap.SugaredLogger
	notifier           Notifier
	stopWatcherChannel chan struct{}

	reloadConsumers []chan bool

	userConfig     *viper.Viper
	internalConfig *viper.Viper
}

// ConnectionInfo groups serial port settings
type ConnectionInfo struct {
	COMPort  string
	BaudRate int
}

const (
	userConfigFilepath     = "config.yaml"
	internalConfigFilepath = "preferences.yaml"

	userConfigName     = "config"
	internalConfigName = "preferences"
	userConfigPath     = "."

	configType              = "yaml"
	configKeySliderMapping  = "slider_mapping"
	configKeyInvertSliders  = "invert_sliders"
	configKeyCOMPort        = "com_port"
	configKeyBaudRate       = "baud_rate"
	configKeyNoiseReduction = "noise_reduction"

	defaultCOMPort  = "COM7"
	defaultBaudRate = 9600
)

var internalConfigPath = path.Join(".", logDirectory)

// Default slider mapping when no configuration is provided
var defaultSliderMapping = func() *sliderMap {
	mapping := newSliderMap()
	mapping.set(0, []string{masterSessionName})
	return mapping
}()

// NewConfig initializes the configuration manager
func NewConfig(logger *zap.SugaredLogger, notifier Notifier) (*CanonicalConfig, error) {
	logger = logger.Named("config")

	cc := &CanonicalConfig{
		logger:             logger,
		notifier:           notifier,
		reloadConsumers:    make([]chan bool, 0),
		stopWatcherChannel: make(chan struct{}),
	}

	cc.initializeViperInstances()
	logger.Debug("Created configuration instance")

	return cc, nil
}

// initializeViperInstances sets up user and internal config
func (cc *CanonicalConfig) initializeViperInstances() {
	cc.userConfig = initializeViper(userConfigName, userConfigPath, map[string]interface{}{
		configKeySliderMapping:  map[string][]string{},
		configKeyInvertSliders:  false,
		configKeyCOMPort:        defaultCOMPort,
		configKeyBaudRate:       defaultBaudRate,
	})
	cc.internalConfig = initializeViper(internalConfigName, internalConfigPath, nil)
}

// initializeViper creates and configures a Viper instance
func initializeViper(name, path string, defaults map[string]interface{}) *viper.Viper {
	config := viper.New()
	config.SetConfigName(name)
	config.SetConfigType(configType)
	config.AddConfigPath(path)

	for key, value := range defaults {
		config.SetDefault(key, value)
	}

	return config
}

// Load reads and validates configuration files
func (cc *CanonicalConfig) Load() error {
	cc.logger.Debugw("Loading user configuration", "path", userConfigFilepath)

	if err := cc.readUserConfig(); err != nil {
		return err
	}
	if err := cc.readInternalConfig(); err != nil {
		cc.logger.Debugw("Skipping optional internal config", "error", err)
	}

	return cc.populateFromVipers()
}

// readUserConfig loads the user-provided configuration
func (cc *CanonicalConfig) readUserConfig() error {
	if !util.FileExists(userConfigFilepath) {
		cc.handleMissingConfig()
		return fmt.Errorf("config file not found: %s", userConfigFilepath)
	}

	if err := cc.userConfig.ReadInConfig(); err != nil {
		return cc.handleConfigError("user config", err)
	}
	return nil
}

// handleMissingConfig notifies the user of missing configuration
func (cc *CanonicalConfig) handleMissingConfig() {
	cc.logger.Warnw("Configuration file not found", "path", userConfigFilepath)
	cc.notifier.Notify("Missing configuration!", fmt.Sprintf(
		"Ensure %s exists in the same directory as deej.", userConfigFilepath))
}

// handleConfigError processes errors during config file loading
func (cc *CanonicalConfig) handleConfigError(configName string, err error) error {
	cc.logger.Warnw("Failed to load configuration", "config", configName, "error", err)

	if strings.Contains(err.Error(), "yaml:") {
		cc.notifier.Notify("Invalid configuration format!",
			"Ensure the YAML file is properly formatted.")
	} else {
		cc.notifier.Notify("Error loading configuration!", "Check logs for more details.")
	}
	return fmt.Errorf("read %s: %w", configName, err)
}

// populateFromVipers reads configuration fields into structured fields
func (cc *CanonicalConfig) populateFromVipers() error {
	cc.SliderMapping = sliderMapFromConfigs(
		cc.userConfig.GetStringMapStringSlice(configKeySliderMapping),
		cc.internalConfig.GetStringMapStringSlice(configKeySliderMapping),
	)
	cc.ConnectionInfo = ConnectionInfo{
		COMPort:  cc.userConfig.GetString(configKeyCOMPort),
		BaudRate: cc.validateBaudRate(cc.userConfig.GetInt(configKeyBaudRate)),
	}
	cc.InvertSliders = cc.userConfig.GetBool(configKeyInvertSliders)
	cc.NoiseReductionLevel = cc.userConfig.GetString(configKeyNoiseReduction)

	cc.logger.Debugw("Configuration populated successfully", "config", cc)
	return nil
}

// validateBaudRate checks for a valid baud rate, returning a default if invalid
func (cc *CanonicalConfig) validateBaudRate(baudRate int) int {
	if baudRate > 0 {
		return baudRate
	}
	cc.logger.Warnw("Invalid baud rate specified, using default", "invalidValue", baudRate, "defaultValue", defaultBaudRate)
	return defaultBaudRate
}