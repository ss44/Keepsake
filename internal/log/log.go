// Package log provides a leveled logger with a debug gate.
package log

import (
	stdlog "log"
	"sync/atomic"
)

var debugEnabled atomic.Bool

// SetDebug enables or disables verbose debug logging.
func SetDebug(enabled bool) {
	debugEnabled.Store(enabled)
}

// Debugf logs only when debug mode is enabled.
func Debugf(format string, args ...interface{}) {
	if debugEnabled.Load() {
		stdlog.Printf("[DEBUG] "+format, args...)
	}
}

// Infof always logs.
func Infof(format string, args ...interface{}) {
	stdlog.Printf("[INFO] "+format, args...)
}

// Errorf always logs as an error.
func Errorf(format string, args ...interface{}) {
	stdlog.Printf("[ERROR] "+format, args...)
}
