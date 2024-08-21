package main

import (
	"errors"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

var (
	instancesLock sync.Mutex
	instances     []*instance

	disallowInstanceCreation = &atomic.Bool{}

	errCreationDisallowed = errors.New("instance creation disallowed")
	errNoPortsDeclared    = errors.New("no ports declared")
	errNoFreePort         = errors.New("no free ports")
)

func allocateNewInstance() (inst *instance, err error) {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	if disallowInstanceCreation.Load() {
		return nil, errCreationDisallowed
	}

	ps, ok := cfg.GetString("ports")
	if !ok {
		return nil, errNoPortsDeclared
	}
	allowed := removeDuplicate(parseNumbersString(ps))
	selected := 0
	for _, p := range allowed {
		used := false
		for _, i := range instances {
			if i.Settings.GamePort == p {
				used = true
				break
			}
		}
		if !used {
			selected = p
			break
		}
	}
	if selected == 0 {
		return nil, errNoFreePort
	}

	inst = &instance{
		Id: newInstanceID(),
		Settings: instanceSettings{
			GamePort: selected,
		},
		commands:       make(chan instanceCommand, 32),
		OnJoinDispatch: map[string]joinDispatch{},
		wg:             sync.WaitGroup{},
	}

	instances = append(instances, inst)

	return
}

func insertInstance(inst *instance) bool {
	if inst == nil {
		log.Println("Inserting nil instance?!")
		return false
	}
	instancesLock.Lock()
	defer instancesLock.Unlock()
	for i := range instances {
		if instances[i].Id == inst.Id {
			return false
		}
		if instances[i].Settings.GamePort == inst.Settings.GamePort {
			return false
		}
	}
	instances = append(instances, inst)
	return true
}

func releaseInstance(inst *instance) {
	if inst == nil {
		log.Println("Releasing nil instance?!")
		return
	}
	instancesLock.Lock()
	instances = slices.DeleteFunc(instances, func(i *instance) bool {
		return i.Id == inst.Id
	})
	instancesLock.Unlock()
}

func routineInstanceCleaner(closechan <-chan struct{}) {
	for {
		select {
		case <-closechan:
			return
		case <-time.After(time.Second * time.Duration(cfg.GetDInt(30, "instanceCleanupTimer"))):
			cleanInstances()
		}
	}
}

func cleanInstances() {
	instancesLock.Lock()
	instances = slices.DeleteFunc(instances, func(i *instance) bool {
		if i.state.Load() == int64(instanceStateExited) {
			log.Printf("Cleaned up instance %d", i.Id)
			return true
		}
		return false
	})
	instancesLock.Unlock()
}

func stopAllRunners() {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	log.Printf("Ordering %d runners to quit processing", len(instances))
	for _, v := range instances {
		if cfg.GetDSBool(false, "shutdownHostsOnExit") {
			v.commands <- instanceCommand{command: icShutdown}
		} else {
			v.commands <- instanceCommand{command: icRunnerStop}
		}
	}
	log.Printf("Waiting for %d runners to quit", len(instances))
	for _, v := range instances {
		v.wg.Wait()
	}
}

func isQueueInLobby(queueName string) int64 {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	return isQueueInLobbyNOLOCK(queueName)
}

func isQueueInLobbyNOLOCK(queueName string) int64 {
	for i := range instances {
		if instances[i].QueueName != queueName {
			continue
		}
		if instances[i].state.Load() <= int64(instanceStateInLobby) {
			return instances[i].Id
		}
	}
	return 0
}

func isInstanceInLobby(instanceID int64) bool {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	return isInstanceInLobbyNOLOCK(instanceID)
}

func isInstanceInLobbyNOLOCK(instanceID int64) bool {
	for i := range instances {
		if instances[i].Id != instanceID {
			continue
		}
		if instances[i].state.Load() <= int64(instanceStateInLobby) {
			return true
		}
	}
	return false
}
