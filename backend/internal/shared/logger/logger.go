package logger

import (
	"log"
	"os"
)

// Logger is an alias used by services for dependency injection.
type Logger = log.Logger

// New returns a standard logger with consistent service prefix.
func New(service string) *Logger {
	return log.New(os.Stdout, "["+service+"] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
}
