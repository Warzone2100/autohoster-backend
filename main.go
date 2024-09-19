package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/natefinch/lumberjack"
)

func main() {
	log.Println("Hello world")
	loadConfig()
	connectToDatabase()

	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: cfg.GetDSString("logs/backend.log", "logs", "filename"),
		MaxSize:  cfg.GetDSInt(10, "logs", "maxsize"),
		Compress: true,
	}))

	go routineDiscordErrorReporter()

	ratelimitChatPenalties = ratelimitChatLoadPenalties()

	recoverInstances()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	closeWebServer := startBackgroundRoutine("web server", routineWebServer)
	closeLobbyKeepalive := startBackgroundRoutine("lobby keepalive", routineLobbyKeepalive)
	closeInstanceCleaner := startBackgroundRoutine("instance cleaner", routineInstanceCleaner)

	log.Println("Autohoster backend started")
	<-signals
	signal.Reset()
	fmt.Println()
	log.Println("Got signal, shutting down...")
	disallowInstanceCreation.Store(true)
	stopAllRunners()
	closeInstanceCleaner()
	closeLobbyKeepalive()
	closeWebServer()
	log.Println("Shutdown complete, bye!")
}
