package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"
	"time"
)

func createPipes(inst *instance) error {
	var err error
	inst.stdin, err = createOpenPipe(path.Join(inst.ConfDir, "stdin.pipe"), os.O_RDWR)
	if err != nil {
		inst.logger.Printf("Error opening stdin pipe: %s", err.Error())
		return err
	}
	inst.stdout, err = createOpenPipe(path.Join(inst.ConfDir, "stdout.pipe"), os.O_RDWR)
	if err != nil {
		inst.logger.Printf("Error opening stdout pipe: %s", err.Error())
		return err
	}
	inst.stderr, err = createOpenPipe(path.Join(inst.ConfDir, "stderr.pipe"), os.O_RDWR)
	if err != nil {
		inst.logger.Printf("Error opening stderr pipe: %s", err.Error())
		return err
	}
	return nil
}

func createOpenPipe(instpath string, flag int) (*os.File, error) {
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	err := syscall.Mkfifo(instpath, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.OpenFile(instpath, flag, os.ModeNamedPipe)
}

func openPipes(inst *instance) error {
	var err error
	inst.stdin, err = os.OpenFile(path.Join(inst.ConfDir, "stdin.pipe"), os.O_RDWR, os.ModeNamedPipe)
	if err != nil {
		inst.logger.Printf("Error opening stdin pipe: %s", err.Error())
		return err
	}
	inst.stdout, err = os.OpenFile(path.Join(inst.ConfDir, "stdout.pipe"), os.O_RDWR, os.ModeNamedPipe)
	if err != nil {
		inst.logger.Printf("Error opening stdout pipe: %s", err.Error())
		return err
	}
	inst.stderr, err = os.OpenFile(path.Join(inst.ConfDir, "stderr.pipe"), os.O_RDWR, os.ModeNamedPipe)
	if err != nil {
		inst.logger.Printf("Error opening stderr pipe: %s", err.Error())
		return err
	}
	return nil
}

func closePipes(inst *instance) error {
	err := inst.stdin.SetDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		inst.logger.Println("Failed to set deadline for stdin:", err)
		return err
	}
	err = inst.stdin.Close()
	if err != nil {
		inst.logger.Println("Failed to close stdin:", err)
		return err
	}
	err = inst.stderr.SetDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		inst.logger.Println("Failed to set deadline for stderr:", err)
		return err
	}
	err = inst.stderr.Close()
	if err != nil {
		inst.logger.Println("Failed to close stderr:", err)
		return err
	}
	err = inst.stdout.SetDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		inst.logger.Println("Failed to set deadline for stdout:", err)
		return err
	}
	err = inst.stdout.Close()
	if err != nil {
		inst.logger.Println("Failed to close stdout:", err)
		return err
	}
	return nil
}

func checkPipeContainsData(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		return 0, errors.New("not a pipe")
	}
	return fi.Size(), nil
}

func drainRemovePipe(p string) error {
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.New("drain: " + err.Error())
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		return errors.New("not a pipe")
	}
	f, err := os.OpenFile(p, os.O_RDWR, os.ModeNamedPipe)
	if err != nil {
		return errors.New("drain: " + err.Error())
	}
	err = f.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		return errors.New("drain: " + err.Error())
	}
	data, err := io.ReadAll(f)
	if err != nil {
		if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrDeadlineExceeded) {
			return errors.New("drain: " + err.Error())
		}
	}
	f.Close()
	if len(data) > 0 {
		perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
		err := os.WriteFile(p+".txt", data, perm)
		if err != nil {
			return errors.New("drain: " + err.Error())
		}
	}
	err = os.Remove(p)
	if err != nil {
		return errors.New("drain: " + err.Error())
	}
	return nil
}
