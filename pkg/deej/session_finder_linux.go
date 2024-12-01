package deej

import (
	"fmt"
	"net"

	"github.com/jfreymuth/pulse/proto"
	"go.uber.org/zap"
)

// paSessionFinder interacts with PulseAudio to discover and manage audio sessions.
type paSessionFinder struct {
	logger        *zap.SugaredLogger
	sessionLogger *zap.SugaredLogger
	client        *proto.Client
	conn          net.Conn
}

// newSessionFinder initializes a new PulseAudio session finder.
func newSessionFinder(logger *zap.SugaredLogger) (SessionFinder, error) {
	client, conn, err := proto.Connect("")
	if err != nil {
		return nil, logAndWrapError(logger, "Failed to establish PulseAudio connection", err)
	}

	request := proto.SetClientName{
		Props: proto.PropList{
			"application.name": proto.PropListString("deej"),
		},
	}
	if err := client.Request(&request, &proto.SetClientNameReply{}); err != nil {
		return nil, logAndWrapError(logger, "Failed to set client name", err)
	}

	sf := &paSessionFinder{
		logger:        logger.Named("session_finder"),
		sessionLogger: logger.Named("sessions"),
		client:        client,
		conn:          conn,
	}

	sf.logger.Debug("Initialized PA session finder instance")
	return sf, nil
}

// GetAllSessions fetches all active audio sessions from PulseAudio.
func (sf *paSessionFinder) GetAllSessions() ([]Session, error) {
	var sessions []Session
	var errors []error

	if masterSink, err := sf.getMasterSinkSession(); err == nil {
		sessions = append(sessions, masterSink)
	} else {
		errors = append(errors, logAndWrapError(sf.logger, "Failed to get master audio sink session", err))
	}

	if masterSource, err := sf.getMasterSourceSession(); err == nil {
		sessions = append(sessions, masterSource)
	} else {
		errors = append(errors, logAndWrapError(sf.logger, "Failed to get master audio source session", err))
	}

	if err := sf.enumerateAndAddSessions(&sessions); err != nil {
		errors = append(errors, logAndWrapError(sf.logger, "Failed to enumerate audio sessions", err))
	}

	if len(errors) > 0 {
		return sessions, fmt.Errorf("encountered errors: %v", errors)
	}
	return sessions, nil
}

// Release releases the PulseAudio session finder resources.
func (sf *paSessionFinder) Release() error {
	defer sf.logger.Debug("Released PA session finder instance")
	return logAndWrapError(sf.logger, "Failed to close PulseAudio connection", sf.conn.Close())
}

// getMasterSinkSession fetches the master sink session.
func (sf *paSessionFinder) getMasterSinkSession() (Session, error) {
	return sf.getMasterSession(proto.GetSinkInfo{}, proto.GetSinkInfoReply{}, true)
}

// getMasterSourceSession fetches the master source session.
func (sf *paSessionFinder) getMasterSourceSession() (Session, error) {
	return sf.getMasterSession(proto.GetSourceInfo{}, proto.GetSourceInfoReply{}, false)
}

// getMasterSession is a helper for fetching master sink/source sessions.
func (sf *paSessionFinder) getMasterSession(req, reply proto.Request, isSink bool) (Session, error) {
	if err := sf.client.Request(&req, &reply); err != nil {
		return nil, fmt.Errorf("get master %v info: %w", getMasterType(isSink), err)
	}

	index := getReplyIndex(reply)
	channels := getReplyChannels(reply)
	return newMasterSession(sf.sessionLogger, sf.client, index, channels, isSink), nil
}

// enumerateAndAddSessions adds all sink input sessions to the provided slice.
func (sf *paSessionFinder) enumerateAndAddSessions(sessions *[]Session) error {
	request := proto.GetSinkInputInfoList{}
	reply := proto.GetSinkInputInfoListReply{}

	if err := sf.client.Request(&request, &reply); err != nil {
		return fmt.Errorf("get sink input list: %w", err)
	}

	for _, info := range reply {
		name, exists := info.Properties["application.process.binary"]
		if !exists {
			sf.logger.Warnw("Missing process name for sink input", "index", info.SinkInputIndex)
			continue
		}
		*sessions = append(*sessions, newPASession(sf.sessionLogger, sf.client, info.SinkInputIndex, info.Channels, name.String()))
	}
	return nil
}

// Helper functions for type abstraction and reuse
func logAndWrapError(logger *zap.SugaredLogger, message string, err error) error {
	if err != nil {
		logger.Warnw(message, "error", err)
	}
	return err
}

func getMasterType(isSink bool) string {
	if isSink {
		return "sink"
	}
	return "source"
}

// Placeholder functions for type handling
func getReplyIndex(reply proto.Request) uint32 {
	// Implement logic for fetching index from reply
	return 0
}

func getReplyChannels(reply proto.Request) uint8 {
	// Implement logic for fetching channels from reply
	return 0
}