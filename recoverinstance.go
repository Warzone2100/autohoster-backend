package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/maxsupermanhd/lac/v2"
)

func recoverInstances() {
	instancesPath, ok := cfg.GetString("instancesPath")
	if !ok {
		log.Fatal("instancesPath not set")
	}
	drs, err := os.ReadDir(instancesPath)
	if err != nil {
		log.Println("Failed to open instances directory, trying to create")
		err = os.MkdirAll(instancesPath, fs.FileMode(cfg.GetDInt(493, "dirPerms")))
		if err != nil {
			log.Fatal("Failed to create instances directory")
		}
	}
	log.Printf("Recovering potential %d instances", len(drs))
	for _, d := range drs {
		if d.Name() == "." || d.Name() == ".." {
			continue
		}
		if !d.IsDir() {
			continue
		}
		confdir := path.Join(instancesPath, d.Name())
		needsArchival := recoverRunner(confdir)
		if needsArchival {
			err := archiveInstance(confdir)
			if err != nil {
				log.Printf("Error archiving instance %q: %s", confdir, err.Error())
			}
		}
	}
}

func recoverRunner(instpath string) bool {
	log.Printf("Recovering instance %q", instpath)
	instid, err := strconv.ParseInt(path.Base(instpath), 10, 64)
	if err != nil {
		log.Printf("Instance path %q does not have valid instance id: %s", instpath, err.Error())
		return false
	}
	inst, err := recoverLoad(path.Join(instpath, "instance.json"))
	if err != nil {
		log.Printf("Instance from path %q failed to load: %s", instpath, err.Error())
		return false
	}
	if inst.Id != instid {
		log.Printf("Instance from path %q has different id (%d) than path (%d)", instpath, inst.Id, instid)
		return false
	}
	if !isPidCmdlineAccurate(inst) {
		log.Printf("Instance from path %q has invalid cmdline, assuming dead", instpath)
		return true
	}
	if !isPidAlive(inst.Pid) {
		log.Printf("Instance from path %q seems to be not alive", instpath)
		return true
	}
	if !insertInstance(inst) {
		log.Printf("Failed to insert instance with id %d", instid)
		return false
	}
	err = openPipes(inst)
	if err != nil {
		log.Printf("Failed to open pipes for instance %q: %s", instpath, err)
		releaseInstance(inst)
		return false
	}
	go instanceRunner(inst)
	return false
}

func isPidCmdlineAccurate(inst *instance) bool {
	cmdbytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", inst.Pid))
	if err != nil {
		log.Printf("err getting proc cmdline: %s", err)
		return false
	}
	cmdline := string(cmdbytes)
	if !strings.Contains(cmdline, fmt.Sprint(inst.Id)) {
		log.Printf("no id")
		return false
	}
	if !strings.Contains(cmdline, "--configdir=") {
		log.Printf("no configdir")
		return false
	}
	if !strings.Contains(cmdline, "--async-join-approve") {
		log.Printf("no async join")
		return false
	}
	recordedCmdlineBytes, err := os.ReadFile(path.Join(inst.ConfDir, "cmdline"))
	if err != nil {
		log.Printf("err getting confdir cmdline: %s %s", inst.ConfDir, err)
		return false
	}
	if cmdline != string(recordedCmdlineBytes) {
		log.Printf("cmdline is not accurate: %q vs %q", cmdline, string(recordedCmdlineBytes))
	}
	return true
}

func isPidAlive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	var (
		rpid   int
		rcomm  string
		rstate rune
	)
	i, err := fmt.Sscanf(string(b), "%d %s %c", &rpid, &rcomm, &rstate)
	if err != nil || i != 3 {
		log.Printf("Failed to parse proc stat: %s", err.Error())
		return false
	}
	switch rstate {
	case 'R': // Running
		return true
	case 'S': // Sleeping in an interruptible wait
		return true
	case 'D': // Waiting in uninterruptible disk sleep
		return true
	case 'W': // Waking
		return true
	case 'I': // Idle
		return true
	default:
		// case 'P': // Parked
		// case 'Z': // Zombie
		// case 'T': // Stopped
		// case 't': // Tracing stop
		// case 'W': // Paging
		// case 'X': // Dead
		// case 'x': // Dead
		// case 'K': // Wakekill
	}
	return false
}

func recoverSave(inst *instance) error {
	if inst == nil {
		return errors.New("inst is nil")
	}
	loadedAtomic := int(inst.state.Load())
	inst.logger.Printf("recoverSave loading atomic: %d", loadedAtomic)
	inst.StateSaved = loadedAtomic
	b, err := json.MarshalIndent(inst, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(inst.ConfDir, "instance.json"), b, fs.FileMode(cfg.GetDInt(493, "filePerms")))
}

func recoverLoad(p string) (*instance, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	inst := &instance{
		commands:       make(chan instanceCommand, 32),
		OnJoinDispatch: map[string]joinDispatch{},
		wg:             sync.WaitGroup{},
	}
	err = json.Unmarshal(b, &inst)
	if err != nil {
		return nil, err
	}
	inst.logger = log.New(log.Writer(), fmt.Sprintf("%d ", inst.Id), log.Flags()|log.Lmsgprefix)
	if inst.Settings.GamePort == 0 {
		return nil, errors.New("loaded instance settings gameport is 0")
	}
	inst.cfgs = []lac.Conf{}
	for _, v := range inst.RestoreCfgs {
		c := lac.NewConf()
		c.CopyTree(v)
		inst.cfgs = append(inst.cfgs, c)
	}
	inst.logger.Printf("atomic state store: %d", int64(inst.StateSaved))
	inst.state.Store(int64(inst.StateSaved))
	inst.recovered = true
	return inst, nil
}
