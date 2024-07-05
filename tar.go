package main

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"os"
)

func tarOpenSeekAppend(p string) (*tar.Writer, *os.File, error) {
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	skipSeek := false
	var f *os.File
	inf, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			skipSeek = true
			f, err = os.OpenFile(p, os.O_RDWR|os.O_CREATE, perm)
			if err != nil {
				return nil, nil, err
			}
		} else {
			return nil, nil, err
		}
	} else {
		if inf.Size() == 0 {
			skipSeek = true
		}
	}

	if f == nil {
		f, err = os.OpenFile(p, os.O_RDWR, perm)
		if err != nil {
			return nil, nil, err
		}
	}

	if !skipSeek {
		if _, err = f.Seek(-2<<9, io.SeekEnd); err != nil {
			f.Close()
			return nil, nil, err
		}
	}
	return tar.NewWriter(f), f, nil
}

func tarAppendFile(tw *tar.Writer, fname string, data []byte) error {
	hdr := &tar.Header{
		Name: fname,
		Size: int64(len(data)),
		Mode: 0777,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	return nil
}
