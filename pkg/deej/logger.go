package deej

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/omriharel/deej/pkg/deej/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	BuildTypeNone    = ""       // Default build type (undefined)
	BuildTypeDev     = "dev"    // Development build type
	BuildTypeRelease = "release" // Release build type

	LogDirectory = "logs"                 // Directory for log files
	LogFilename  = "deej-latest-run.log"  // Default log file name
)

// NewLogger initializes and returns a new logger instance based on the build type.
// - For release builds, logs to a file with info level and above.
// - For development builds, logs to stderr with debug level and colorful output.
func NewLogger(buildType string) (*zap.SugaredLogger, error) {
	var loggerConfig zap.Config

	// Configure for release builds: logs to file, "info" level and above
	if buildType == BuildTypeRelease {
		// Ensure the log directory exists
		if err := util.EnsureDirExists(LogDirectory); err != nil {
			return nil, fmt.Errorf("failed to create log directory %s: %w", LogDirectory, err)
		}

		// Set production configuration
		loggerConfig = zap.NewProductionConfig()
		loggerConfig.OutputPaths = []string{filepath.Join(LogDirectory, LogFilename)}
		loggerConfig.Encoding = "console"

	} else {
		// Configure for development builds: logs to stderr, "debug" level and colorful output
		loggerConfig = zap.NewDevelopmentConfig()
		loggerConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Common encoder settings: human-readable timestamps and aligned names
	loggerConfig.EncoderConfig.EncodeCaller = nil // Disable caller encoding
	loggerConfig.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
	}
	loggerConfig.EncoderConfig.EncodeName = func(name string, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(fmt.Sprintf("%-27s", name))
	}

	// Build the logger
	logger, err := loggerConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Return the sugared logger for ease of use
	return logger.Sugar(), nil
}