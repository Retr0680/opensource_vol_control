package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/omriharel/deej/pkg/deej"
)

var (
	gitCommit  string
	versionTag string
	buildType  string
	verbose    bool
)

func init() {
	// Consolidate verbose flag definition
	flag.BoolVar(&verbose, "verbose", false, "Show verbose logs (useful for debugging serial)")
	flag.BoolVar(&verbose, "v", false, "Shorthand for --verbose")
	flag.Parse()
}

func main() {
	// First we need a logger
	logger, err := deej.NewLogger(buildType)
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		os.Exit(1) // Exit with error status
	}

	// Named logger for the 'main' function
	named := logger.Named("main")
	named.Debug("Created logger")

	// Log version info
	if versionTag != "" || gitCommit != "" {
		named.Infow("Version info", "gitCommit", gitCommit, "versionTag", versionTag, "buildType", buildType)
	}

	// Provide a fair warning if the user's running in verbose mode
	if verbose {
		named.Debug("Verbose mode enabled, all log messages will be shown")
		// Set logger to verbose level for more detailed logging if needed
	}

	// Create the deej instance
	d, err := deej.NewDeej(logger, verbose)
	if err != nil {
		named.Fatalw("Failed to create deej instance", "error", err)
	}

	// Set version info for tray (if available)
	if versionTag != "" || gitCommit != "" {
		versionIdentifier := versionTag
		if versionIdentifier == "" {
			versionIdentifier = gitCommit
		}
		versionString := fmt.Sprintf("Version %s-%s", buildType, versionIdentifier)
		d.SetVersion(versionString)
	}

	// Initialize deej
	if err := d.Initialize(); err != nil {
		named.Fatalw("Failed to initialize deej", "error", err)
	}

	named.Info("Deej initialized successfully")
}