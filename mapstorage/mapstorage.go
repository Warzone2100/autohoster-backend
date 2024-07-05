package mapstorage

import (
	"io/fs"
	"os"
	"path"
	"sync"

	mapsdatabase "github.com/maxsupermanhd/go-wz/maps-database"
	"github.com/maxsupermanhd/lac/v2"
)

type Mapstorage struct {
	fslock sync.Mutex
	cfg    lac.Conf
}

func NewMapstorage(cfg lac.Conf) (*Mapstorage, error) {
	m := &Mapstorage{cfg: cfg}
	return m, os.MkdirAll(m.getRoot(), fs.FileMode(m.cfg.GetDInt(766, "dirPerms")))
}

func (m *Mapstorage) getRoot() string {
	return m.cfg.GetDSString("maps", "root")
}

func (m *Mapstorage) getMapPath(hash string) string {
	return path.Join(m.getRoot(), hash+".wz")
}

func (m *Mapstorage) GetMap(hash string) ([]byte, error) {
	p := m.getMapPath(hash)
	m.fslock.Lock()
	ret, err := os.ReadFile(p)
	m.fslock.Unlock()
	if err == nil {
		return ret, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	ret, err = mapsdatabase.FetchMapBlob(hash)
	if err != nil {
		return nil, err
	}
	m.fslock.Lock()
	err = os.WriteFile(p, ret, fs.FileMode(m.cfg.GetDInt(644, "filePerms")))
	m.fslock.Unlock()
	return ret, err
}
