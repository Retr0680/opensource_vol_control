package deej

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

// SerialIO provides a deej-aware abstraction layer for managing serial I/O
type SerialIO struct {
	comPort  string
	baudRate uint

	deej   *Deej
	logger *zap.SugaredLogger

	stopChannel chan bool
	connected   bool
	connOptions serial.OpenOptions
	conn        io.ReadWriteCloser

	lastKnownNumSliders        int
	currentSliderPercentValues []float32

	sliderMoveConsumers []chan SliderMoveEvent
}

// SliderMoveEvent represents a single slider movement captured by deej
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

var expectedLinePattern = regexp.MustCompile(`^\d{1,4}(\|\d{1,4})*\r\n$`)

// NewSerialIO creates a new SerialIO instance
func NewSerialIO(deej *Deej, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	sio := &SerialIO{
		deej:                deej,
		logger:              logger,
		stopChannel:         make(chan bool),
		connected:           false,
		conn:                nil,
		sliderMoveConsumers: []chan SliderMoveEvent{},
	}

	logger.Debug("Created SerialIO instance")
	sio.setupOnConfigReload()

	return sio, nil
}

// Start attempts to establish a serial connection
func (sio *SerialIO) Start() error {
	if sio.connected {
		sio.logger.Warn("Connection already active, cannot start a new one")
		return errors.New("serial: connection already active")
	}

	minimumReadSize := 0
	if util.Linux() {
		minimumReadSize = 1
	}

	sio.connOptions = serial.OpenOptions{
		PortName:        sio.deej.config.ConnectionInfo.COMPort,
		BaudRate:        uint(sio.deej.config.ConnectionInfo.BaudRate),
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: uint(minimumReadSize),
	}

	sio.logger.Debugw("Opening serial connection",
		"comPort", sio.connOptions.PortName,
		"baudRate", sio.connOptions.BaudRate,
		"minReadSize", minimumReadSize)

	conn, err := serial.Open(sio.connOptions)
	if err != nil {
		sio.logger.Warnw("Failed to open serial connection", "error", err)
		return fmt.Errorf("open serial connection: %w", err)
	}

	sio.conn = conn
	sio.connected = true
	sio.logger.Infow("Serial connection established", "port", sio.connOptions.PortName)

	go sio.readLoop()

	return nil
}

// Stop shuts down the serial connection if active
func (sio *SerialIO) Stop() {
	if sio.connected {
		sio.logger.Debug("Closing serial connection")
		sio.stopChannel <- true
	} else {
		sio.logger.Debug("No active connection to stop")
	}
}

// SubscribeToSliderMoveEvents allows listeners to subscribe to slider movement events
func (sio *SerialIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	sio.sliderMoveConsumers = append(sio.sliderMoveConsumers, ch)
	return ch
}

// setupOnConfigReload listens for configuration changes and adjusts the connection as needed
func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.deej.config.SubscribeToChanges()
	const stopDelay = 50 * time.Millisecond

	go func() {
		for {
			select {
			case <-configReloadedChannel:
				go func() {
					time.Sleep(stopDelay)
					sio.lastKnownNumSliders = 0
				}()

				if sio.needsReconnect() {
					sio.logger.Info("Config change detected, reconnecting")
					sio.Stop()

					time.Sleep(stopDelay)

					if err := sio.Start(); err != nil {
						sio.logger.Warnw("Failed to reconnect", "error", err)
					} else {
						sio.logger.Debug("Reconnection successful")
					}
				}
			}
		}
	}()
}

// readLoop continuously reads data from the serial connection
func (sio *SerialIO) readLoop() {
	reader := bufio.NewReader(sio.conn)

	for {
		select {
		case <-sio.stopChannel:
			sio.closeConnection()
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				sio.logger.Warnw("Failed to read from serial", "error", err)
				sio.closeConnection()
				return
			}
			sio.processLine(strings.TrimSuffix(line, "\r\n"))
		}
	}
}

// processLine parses a line of slider data and triggers events
func (sio *SerialIO) processLine(line string) {
	if !expectedLinePattern.MatchString(line) {
		return
	}

	values := strings.Split(line, "|")
	numSliders := len(values)

	if numSliders != sio.lastKnownNumSliders {
		sio.logger.Infow("Slider count updated", "count", numSliders)
		sio.lastKnownNumSliders = numSliders
		sio.currentSliderPercentValues = make([]float32, numSliders)
		for i := range sio.currentSliderPercentValues {
			sio.currentSliderPercentValues[i] = -1.0
		}
	}

	var events []SliderMoveEvent
	for i, val := range values {
		rawValue, err := strconv.Atoi(val)
		if err != nil || rawValue > 1023 {
			sio.logger.Debugw("Invalid slider value", "value", val, "line", line)
			return
		}

		scaledValue := util.NormalizeScalar(float32(rawValue) / 1023.0)
		if sio.deej.config.InvertSliders {
			scaledValue = 1 - scaledValue
		}

		if util.SignificantlyDifferent(sio.currentSliderPercentValues[i], scaledValue, sio.deej.config.NoiseReductionLevel) {
			sio.currentSliderPercentValues[i] = scaledValue
			events = append(events, SliderMoveEvent{i, scaledValue})
		}
	}

	for _, event := range events {
		for _, ch := range sio.sliderMoveConsumers {
			ch <- event
		}
	}
}

// closeConnection handles the safe closure of the serial connection
func (sio *SerialIO) closeConnection() {
	if sio.conn != nil {
		if err := sio.conn.Close(); err != nil {
			sio.logger.Warnw("Error closing serial connection", "error", err)
		} else {
			sio.logger.Debug("Serial connection closed")
		}
	}
	sio.conn = nil
	sio.connected = false
}

// needsReconnect checks if the connection parameters have changed
func (sio *SerialIO) needsReconnect() bool {
	return sio.deej.config.ConnectionInfo.COMPort != sio.connOptions.PortName ||
		uint(sio.deej.config.ConnectionInfo.BaudRate) != sio.connOptions.BaudRate
}