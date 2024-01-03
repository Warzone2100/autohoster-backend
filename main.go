package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxsupermanhd/go-wz/lobby"
	"github.com/maxsupermanhd/lac"
)

var (
	cfg *lac.Conf
)

func main() {
	log.Println("Hello world")
	loadConfig()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Starting lobby lookup")
	closeLobbyKeepalive := startLobbyKeepalive()

	log.Println("Autohoster backend started")
	<-signals
	fmt.Println()
	log.Println("Got signal, shutting down...")
	closeLobbyKeepalive()
	log.Println("Shutdown complete, bye!")
}

func startLobbyKeepalive() func() {
	closechan := make(chan bool)
	go lobbyKeepalive(closechan)
	return func() {
		log.Println("Shutting down lobby keepalive routine")
		closechan <- true
		log.Println("Lobby keepalive shutdown")
	}
}

func lobbyKeepalive(closechan chan bool) {
	interval := time.Duration(cfg.GetDSInt(10, "keepaliveInterval")) * time.Second
	for {
		select {
		case <-closechan:
			return
		case <-time.After(interval):
		}
		resp, err := lobby.LobbyLookup()
		if err != nil {
			log.Printf("Failed to lookup lobby: %s", err.Error())
		}
		log.Printf("Lobby has %d rooms", len(resp.Rooms))
	}
}
