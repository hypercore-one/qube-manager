package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/nbd-wtf/go-nostr"
	"gopkg.in/natefinch/lumberjack.v2"
)

// setupLogging initializes logging to both stdout and a rotating file in configDir
func setupLogging(configDir string) {
	logFile := filepath.Join(configDir, "manager.log")
	multi := io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10,   // megabytes
		MaxBackups: 3,    // number of backup files
		MaxAge:     28,   // days
		Compress:   true, // compress backups
	})
	log.SetOutput(multi)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func configureNostrLogging(verbose bool) {
	if !verbose {
		// Silence all nostr logs by sending them to io.Discard
		nostr.InfoLogger = log.New(io.Discard, "", 0)
	}
}
