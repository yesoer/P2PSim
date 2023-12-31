package log

import (
	"flag"
	"fmt"
	"sync"
)

var LogLvlFlag = flag.Int("log-level", int(DebugLevel), "set loglevels info, error, debug with 1,2,3")

const (
	resetColor = "\033[0m"
	redColor   = "\033[31m"
	greenColor = "\033[32m"
	blueColor  = "\033[34m"
)

// LogLevel represents the log level
type LogLevel int

const (
	// InfoLevel represents the info log level
	InfoLevel LogLevel = iota
	// ErrorLevel represents the error log level
	ErrorLevel
	// DebugLevel represents the debug log level
	DebugLevel
)

var mu = sync.Mutex{}

// log prints the message with the specified color
func log(colorCode, level string, logLevel LogLevel, optionalErr error, message ...any) {
	if *LogLvlFlag < int(logLevel) {
		return
	}

	logPrefix := "%s[%s]%s "
	if logLevel == ErrorLevel {
		mu.Lock()
		fmt.Printf(logPrefix+"%+v\n", colorCode, level, resetColor, optionalErr)
		fmt.Println(message...)
		mu.Unlock()
		return
	}

	mu.Lock()
	fmt.Printf(logPrefix, colorCode, level, resetColor)
	fmt.Println(message...)
	mu.Unlock()
}

// Info logs information messages, so anything that may be interesting to the
// end user : application health, system resources etc.
func Info(message ...any) {
	log(greenColor, "INFO", InfoLevel, nil, message...)
}

// Error logs error messages
func Error(err error, message ...any) {
	log(redColor, "ERROR", ErrorLevel, err, message...)
}

// Debug logs debug messages, so anything that gives information about specific
// variables and data flow within the application
func Debug(message ...any) {
	log(blueColor, "DEBUG", DebugLevel, nil, message...)
}
