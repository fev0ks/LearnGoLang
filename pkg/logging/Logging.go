package logging

import (
	"fmt"
	"time"
)

type MyInterface interface {
	SetDebug(debug bool)
	GetDebug() bool
}

type MyInterface2 interface {
	SetDebug(debug bool)
	GetDebug() bool
}

type Logger struct {
	timeFormat string
	debug      bool
}

type BigLogger struct {
	Logger
	logger Logger
}
type CompareFunc func(a, b interface{}) int

func New(timeFormat string, debug bool) *Logger {
	return &Logger{
		timeFormat: timeFormat,
		debug:      debug, //there is ',' because after ',' compiler doesn't add ';'
	}
}

func (logger Logger) Log(level *LoggerLevel, message string) {
	switch level {
	case Info, Debug:
		if logger.debug {
			logger.write(level.Level, message)
		} else {
			logger.write(level.Level, "Debug is turned off; "+message)
		}
	default:
		logger.write(level.Level, message)
	}
}

func PrintLoggers(loggers []*Logger) {
	for i, logger := range loggers {
		logger.write(Info.Level, fmt.Sprintf("I'm logger[%v] = %v", i, logger))
	}
}

func (logger *Logger) SwitchDebug() *Logger {
	logger.debug = !logger.debug
	return logger
}

func (logger Logger) SetDebug(debug bool) {
	logger.debug = debug
}

func GetDebugInt(logger MyInterface) bool {
	return logger.GetDebug()
}

func (logger *Logger) GetDebug() bool {
	return logger.debug
}

func (logger *Logger) write(level string, s string) {
	fmt.Printf("[%s] %s %s\n", level, time.Now().Format(logger.timeFormat), s)
}
