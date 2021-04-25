package logging

type LoggerLevel struct {
	Level string
}

// logging.Info = logging.Debug - not constants

var Info = newLoggerLevel("Info")
var Error = newLoggerLevel("Error")
var Warn = newLoggerLevel("Warn")
var Debug = newLoggerLevel("Debug")

func newLoggerLevel(name string) *LoggerLevel {
	return &LoggerLevel{
		Level: name,
	}
}
