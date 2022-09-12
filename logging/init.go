package logging

import (
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
)

// InitLog Initialize logging settings
func InitLog(logFilename string, lev string) {
	log.Debugf("LOG filename: %s, level: %s", logFilename, lev)
	if logFilename == "" {
		path, _ := os.Executable()
		_, exec := filepath.Split(path)
		logFilename = exec + ".log"
		log.Warning("LOG filename is empty, log info save in current path($EXECUTABLE_NAME.log)")
	}

	// Set to generate a logging file every day
	// Keep logs for 15 days
	writer, _ := rotatelogs.New(logFilename+".%Y%m%d",
		rotatelogs.WithLinkName(logFilename),
		rotatelogs.WithRotationCount(15),
		rotatelogs.WithRotationTime(time.Duration(24)*time.Hour))

	log.SetOutput(writer)

	level, err := log.ParseLevel(lev)
	if err == nil {
		log.SetLevel(level)
	}
}
