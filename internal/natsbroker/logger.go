package natsbroker

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/centrifugal/centrifuge"
)

type LogAdapter struct {
	node *centrifuge.Node
}

func (l *LogAdapter) Noticef(format string, v ...interface{}) {
	l.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf(format, v...)))
}

func (l *LogAdapter) Warnf(format string, v ...interface{}) {
	l.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, fmt.Sprintf(format, v...)))
}

func (l *LogAdapter) Fatalf(format string, v ...interface{}) {
	log.Fatal().Msgf(format, v...)
}

func (l *LogAdapter) Errorf(format string, v ...interface{}) {
	l.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, fmt.Sprintf(format, v...)))
}

func (l *LogAdapter) Debugf(format string, v ...interface{}) {
	l.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, fmt.Sprintf(format, v...)))
}

func (l *LogAdapter) Tracef(format string, v ...interface{}) {
	l.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelTrace, fmt.Sprintf(format, v...)))
}
