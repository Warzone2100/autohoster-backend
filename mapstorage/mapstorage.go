package mapstorage

import (
	"os"
	"path"

	mapsdatabase "github.com/maxsupermanhd/go-wz/maps-database"
	"github.com/maxsupermanhd/lac"
)

type Mapstorage struct {
	cfg *lac.ConfSubtree
}

func NewMapstorage(cfg *lac.ConfSubtree) *Mapstorage {
	return &Mapstorage{
		cfg: cfg,
	}
}

func (m *Mapstorage) getMapPath(hash string) string {
	root := m.cfg.GetDSString("maps", "root")
	return path.Join(root, hash+".wz")
}

func (m *Mapstorage) GetMap(hash string) ([]byte, error) {
	p := m.getMapPath(hash)
	ret, err := os.ReadFile(p)
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
	return ret, os.WriteFile(p, ret, 0644)
}
