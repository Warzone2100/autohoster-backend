package main

import (
	"log"
	"sort"
	"time"

	"github.com/maxsupermanhd/go-wz/lobby"
)

func routineLobbyKeepalive(closechan <-chan struct{}) {
	interval := time.Duration(cfg.GetDSInt(5, "lobbyPollInterval")) * time.Second
	for {
		resp, err := lobby.LobbyLookup()
		if err != nil {
			log.Printf("Failed to lookup lobby: %s", err.Error())
		}
		log.Printf("Lobby has %d rooms", len(resp.Rooms))
		populateLobby(resp.Rooms)
		select {
		case <-closechan:
			return
		case <-time.After(interval):
		}
	}
}

func populateLobby(lr []lobby.LobbyRoom) {
	if !cfg.GetDSBool(false, "allowSpawn") {
		log.Println("Room spawning disabled")
		return
	}
	maxlobby := cfg.GetDSInt(8, "spawnCutoutLobbyRooms")
	if len(lr) >= maxlobby {
		log.Printf("Queue processing paused, too many rooms in lobby (%d >= %d)", len(lr), maxlobby)
		return
	}
	maxrunning := cfg.GetDSInt(18, "spawnCutoutRunningRooms")
	runningRooms := 0
	instancesLock.Lock()
	for _, v := range instances {
		if v.state.Load() == int64(instanceStateInGame) {
			runningRooms++
		}
	}
	instancesLock.Unlock()
	if runningRooms >= maxrunning {
		log.Printf("Queue processing paused, too many running rooms (%d >= %d)", runningRooms, maxlobby)
		return
	}

	queuesK, ok := cfg.GetKeys("queues")
	if !ok {
		log.Println("Queue processing paused, queues not defined in config")
		return
	}

	sort.Strings(queuesK)

	for _, queueName := range queuesK {
		if cfg.GetDSBool(false, "queues", queueName, "disabled") {
			continue
		}
		li := isInstanceInLobby(queueName)
		if li != 0 {
			log.Printf("Queue %q in lobby with instance id %v", queueName, li)
			continue
		}
		log.Printf("Queue %q is missing from lobby, spawning new one...", queueName)
		gi, err := generateInstance(cfg.DupSubTree("queues", queueName))
		if err != nil {
			log.Printf("Failed to generate instance: %s", err.Error())
			if gi != nil {
				releaseInstance(gi)
			}
			continue
		}
		gi.QueueName = queueName
		// log.Printf("Generated instance: %s", spew.Sdump(gi))
		go spawnRunner(gi)
	}
}
