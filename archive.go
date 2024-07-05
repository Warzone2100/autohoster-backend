package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	archiveLock sync.Mutex
)

func doesConfdirPathMakeSense(confdirPath string) bool {
	instanceIdString := path.Base(confdirPath)
	num, err := strconv.ParseInt(instanceIdString, 10, 64)
	if err != nil {
		return false
	}
	return num > 1593464400 // too early to process (Mon Jun 29 2020 21:00:00 GMT+0000)
}

func archiveInstance(confdirPath string) error {
	log.Printf("Archiving %q...", confdirPath)
	archiveLock.Lock()
	defer archiveLock.Unlock()

	if !doesConfdirPathMakeSense(confdirPath) {
		return fmt.Errorf("path %q does not make any sense", confdirPath)
	}

	log.Printf("Archiving %q, dumping pipes...", confdirPath)
	err := archiveInstanceDumpPipes(confdirPath)
	if err != nil {
		return errors.New("dumping pipes: " + err.Error())
	}

	log.Printf("Archiving %q, filling archive...", confdirPath)
	err = archiveInstanceAppendTree(confdirPath)
	if err != nil {
		return errors.New("appending to tar: " + err.Error())
	}

	log.Printf("Archiving %q, removing instance directory...", confdirPath)
	err = os.RemoveAll(confdirPath)
	if err != nil {
		return errors.New("removing directory: " + err.Error())
	}

	return nil
}

func archiveInstanceDumpPipes(confdirPath string) error {
	var wg sync.WaitGroup
	wg.Add(3)
	drainerrors := make(chan error, 10)
	draintask := func(pipename string) {
		err := drainRemovePipe(path.Join(confdirPath, pipename+".pipe"))
		if err != nil {
			drainerrors <- errors.New(pipename + ": " + err.Error())
		}
	}
	go func() { draintask("stdin"); wg.Done() }()
	go func() { draintask("stdout"); wg.Done() }()
	go func() { draintask("stderr"); wg.Done() }()
	wg.Wait()
	select {
	case err := <-drainerrors:
		return err
	default:
	}
	return nil
}

func archiveInstanceIdToWeek(p string) int64 {
	num, err := strconv.ParseInt(p, 10, 64)
	if err != nil {
		return -1
	}
	return num / (7 * 24 * 60 * 60)
}

func archiveInstanceAppendTree(confdirPath string) error {
	archivesDir, ok := cfg.GetString("archivesPath")
	if !ok {
		return errors.New("no archivesPath in config")
	}
	weekId := archiveInstanceIdToWeek(path.Base(confdirPath))
	if weekId == -1 {
		return errors.New("week id returned -1")
	}
	tw, f, err := tarOpenSeekAppend(path.Join(archivesDir, fmt.Sprintf("%d.tar", weekId)))
	if err != nil {
		return errors.New("opening tar: " + err.Error())
	}

	removePrefix := path.Dir(confdirPath)

	filepath.Walk(confdirPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if info.IsDir() {
			if info.Name() == "cache" {
				return filepath.SkipDir
			}
			return nil
		}
		apath, ok := strings.CutPrefix(path, removePrefix)
		if !ok {
			return fmt.Errorf("unable to cut prefix %q from path %q", removePrefix, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return tarAppendFile(tw, apath, data)
	})

	if err := tw.Close(); err != nil {
		return errors.New("closing wrapper: " + err.Error())
	}
	return f.Close()
}
