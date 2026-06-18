package internal

import (
	"log"
	"os"
)

var logger *log.Logger

func InitLogger() {
	logger = log.New(os.Stdout, "", log.LstdFlags)
}

func LogInfo(format string, args ...interface{}) {
	if logger != nil {
		logger.Printf("[INFO] "+format, args...)
	}
}

func LogWarn(format string, args ...interface{}) {
	if logger != nil {
		logger.Printf("[WARN] "+format, args...)
	}
}

func LogError(format string, args ...interface{}) {
	if logger != nil {
		logger.Printf("[ERROR] "+format, args...)
	}
}
