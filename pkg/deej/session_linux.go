package deej

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
	"github.com/jfreymuth/pulse/proto"
)

// Constants
const maxVolume = 0x10000
const sessionCreationLogMessage = "Creating audio session"

// Predefined error
var errNoSuchProcess = errors.New("no such process")

// paSession represents a PulseAudio session for a specific process.
type paSession struct {
	baseSession
	processName       string
	client           *proto.Client
	sinkInputIndex   uint32
	sinkInputChannels byte
}

// masterSession represents a master audio session (either input or output).
type masterSession struct {
	baseSession
	client          *proto.Client
	streamIndex     uint32
	streamChannels  byte
	isOutput        bool
}

func newPASession(
	logger *zap.SugaredLogger,
	client *proto.Client,
	sinkInputIndex uint32,
	sinkInputChannels byte,
	processName string,
) *paSession {
	s := &paSession{
		client:            client,
		sinkInputIndex:    sinkInputIndex,
		sinkInputChannels: sinkInputChannels,
		processName:       processName,
		name:              processName,
		humanReadableDesc: processName,
	}
	s.logger = logger.Named(s.Key())
	s.logger.Debugw(sessionCreationLogMessage, "session", s)
	return s
}

func newMasterSession(
	logger *zap.SugaredLogger,
	client *proto.Client,
	streamIndex uint32,
	streamChannels byte,
	isOutput bool,
) *masterSession {
	key := masterSessionName
	if !isOutput {
		key = inputSessionName
	}

	s := &masterSession{
		client:         client,
		streamIndex:    streamIndex,
		streamChannels: streamChannels,
		isOutput:       isOutput,
		name:           key,
		humanReadableDesc: key,
	}

	s.logger = logger.Named(key)
	s.logger.Debugw(sessionCreationLogMessage, "session", s)
	return s
}

// GetVolume retrieves the current volume for the session.
func (s *paSession) GetVolume() float32 {
	return getVolumeFromClient(s.client, s.sinkInputIndex, s.sinkInputChannels, s.logger)
}

// SetVolume sets the volume for the session.
func (s *paSession) SetVolume(v float32) error {
	volumes := createChannelVolumes(s.sinkInputChannels, v)
	request := proto.SetSinkInputVolume{
		SinkInputIndex: s.sinkInputIndex,
		ChannelVolumes: volumes,
	}
	if err := s.client.Request(&request, nil); err != nil {
		return fmt.Errorf("adjust session volume: %w", err)
	}
	s.logger.Debugw("Adjusting session volume", "to", fmt.Sprintf("%.2f", v))
	return nil
}

// Release releases the audio session resources.
func (s *paSession) Release() {
	s.logger.Debug("Releasing audio session")
}

// String provides a string representation of the session.
func (s *paSession) String() string {
	return fmt.Sprintf(sessionStringFormat, s.humanReadableDesc, s.GetVolume())
}

// GetVolume retrieves the current volume for the master session.
func (s *masterSession) GetVolume() float32 {
	return getVolumeFromClient(s.client, s.streamIndex, s.streamChannels, s.logger)
}

// SetVolume sets the volume for the master session.
func (s *masterSession) SetVolume(v float32) error {
	var request proto.RequestArgs
	volumes := createChannelVolumes(s.streamChannels, v)
	if s.isOutput {
		request = &proto.SetSinkVolume{
			SinkIndex:      s.streamIndex,
			ChannelVolumes: volumes,
		}
	} else {
		request = &proto.SetSourceVolume{
			SourceIndex:    s.streamIndex,
			ChannelVolumes: volumes,
		}
	}
	if err := s.client.Request(request, nil); err != nil {
		return fmt.Errorf("adjust session volume: %w", err)
	}
	s.logger.Debugw("Adjusting session volume", "to", fmt.Sprintf("%.2f", v))
	return nil
}

// Release releases the master session resources.
func (s *masterSession) Release() {
	s.logger.Debug("Releasing audio session")
}

// String provides a string representation of the master session.
func (s *masterSession) String() string {
	return fmt.Sprintf(sessionStringFormat, s.humanReadableDesc, s.GetVolume())
}

// Helper function to avoid code duplication for getting volume
func getVolumeFromClient(client *proto.Client, index uint32, channels byte, logger *zap.SugaredLogger) float32 {
	var level float32
	var request proto.RequestArgs
	var reply proto.RequestReply

	if channels > 0 {
		// Construct request based on input or output session type
		switch {
		case isSinkIndex(index):
			request = &proto.GetSinkInputInfo{SinkInputIndex: index}
			reply = &proto.GetSinkInputInfoReply{}
		case isSourceIndex(index):
			request = &proto.GetSourceInfo{SourceIndex: index}
			reply = &proto.GetSourceInfoReply{}
		}
		if err := client.Request(request, &reply); err != nil {
			logger.Warnw("Failed to get session volume", "error", err)
			return 0
		}
		level = parseChannelVolumes(reply.GetChannelVolumes())
	}
	return level
}

// Helper function to create channel volumes based on the volume level
func createChannelVolumes(channels byte, volume float32) []uint32 {
	volumes := make([]uint32, channels)
	for i := range volumes {
		volumes[i] = uint32(volume * maxVolume)
	}
	return volumes
}

// Helper function to parse channel volumes into a float value
func parseChannelVolumes(volumes []uint32) float32 {
	var total uint32
	for _, volume := range volumes {
		total += volume
	}
	return float32(total) / float32(len(volumes)) / float32(maxVolume)
}

// Utility functions for index validation (to differentiate sinks and sources)
func isSinkIndex(index uint32) bool {
	// Implement logic to identify sink index
	return true
}

func isSourceIndex(index uint32) bool {
	// Implement logic to identify source index
	return true
}