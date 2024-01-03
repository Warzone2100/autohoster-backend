package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

type instance struct {
	binpath      string
	confdir      string
	gameport     int
	startplayers int
	id           int64
	timelimit    int
	admins       []string
	shouldClose  chan bool
	closed       chan bool
}

func spawnInstance(plan instance) {
	args := []string{
		"--configdir=" + plan.confdir,
		"--noshadows",
		"--nosound",
		"--host",
		"--notexturecompression",
		"--headless",
		"--gameport=" + fmt.Sprint(plan.gameport),
		"--enablelobbyslashcmd",
		"--startplayers=" + fmt.Sprint(plan.startplayers),
		"--gamelog-output=cmdinterface",
		"--gamelog-outputkey=playerposition",
		"--gamelog-outputnaming=autohosterclassic",
		"--gamelog-frameinterval=1",
		"--gametimelimit=" + fmt.Sprint(plan.confdir),
		"--host-chat-config=quickchat",
		"--async-join-approve",
	}

	sock := cfg.GetDSString("./sockets/", "socketsPath") + fmt.Sprint(plan.id)
	args = append(args, "--enablecmdinterface=unixsocket:"+sock)

	pid, err := syscall.ForkExec(plan.binpath, args, &syscall.ProcAttr{
		Dir:   "",
		Env:   []string{},
		Files: []uintptr{pipeNullRead(), pipeNullWrite(), pipeNullWrite()},
		Sys: &syscall.SysProcAttr{
			Setsid:  true,
			Setpgid: true,
			Noctty:  true,
		},
	})
	if err != nil {
		log.Printf("ForkExec failed: %s", err)
		return
	}
	log.Printf("Spawned new instance with PID %d", pid)
}

func pipeNullRead() uintptr {
	nullf, err := os.OpenFile("/dev/null", os.O_RDONLY, 0)
	if err != nil {
		log.Printf("Failed to open /dev/null: %s", err)
		return 0
	}
	return nullf.Fd()
}

func pipeNullWrite() uintptr {
	nullf, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		log.Printf("Failed to open /dev/null: %s", err)
		return 0
	}
	return nullf.Fd()
}
