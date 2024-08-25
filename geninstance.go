package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/maxsupermanhd/lac/v2"
)

func generateInstance(instcfg lac.Conf) (inst *instance, err error) {
	inst, err = allocateNewInstance()
	if err != nil {
		return
	}
	inst.logger = log.New(log.Writer(), fmt.Sprintf("%d ", inst.Id), log.Flags()|log.Lmsgprefix)
	inst.cfg = instcfg

	inst.ConfDir = geniConfdir(inst)
	err = makeDirs(fs.FileMode(cfg.GetDInt(755, "dirPerms")), []string{
		path.Join(inst.ConfDir, "maps"),
		path.Join(inst.ConfDir, "autohost"),
		path.Join(inst.ConfDir, "multiplay", "players"),
	})
	if err != nil {
		return
	}

	err = geniMap(inst)
	if err != nil {
		return
	}

	inst.cfgs = []lac.Conf{
		inst.cfg.DupSubTree("maps", inst.Settings.MapName),
		inst.cfg,
		cfg.DupSubTree("settingsFallback"),
	}

	inst.RestoreCfgs = []map[string]any{}
	for i, v := range inst.cfgs {
		m, ok := v.GetMapStringAny()
		if !ok {
			inst.logger.Printf("Failed to copy %d map string any for restore cfgs!", i)
			continue
		}
		inst.RestoreCfgs = append(inst.RestoreCfgs, m)
	}

	inst.Admins, inst.AdminsPolicy = geniAdminspolicy(inst)

	err = geniPreset(inst)
	if err != nil {
		return
	}
	err = geniBanlist(inst)
	if err != nil {
		return
	}
	err = geniConfig(inst)
	if err != nil {
		return
	}
	err = geniActions(inst)
	return
}

func geniConfig(inst *instance) error {
	vals := map[string]string{}
	for _, v := range inst.cfgs {
		setKeys, ok := v.GetKeys("config")
		if !ok {
			continue
		}
		for _, setKey := range setKeys {
			setVal, ok := v.GetString("config", setKey)
			if !ok {
				delete(vals, setKey)
			} else {
				vals[setKey] = setVal
			}
		}
	}
	c := "[General]\n"
	for k, v := range vals {
		c += fmt.Sprintf("%s=%s\n", k, v)
	}
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	return os.WriteFile(path.Join(inst.ConfDir, "config"), []byte(c), perm)
}

func geniActions(inst *instance) error {
	acts := map[string]map[string]any{}
	for _, v := range inst.cfgs {
		actionKeys, ok := v.GetKeys("actions")
		if !ok {
			continue
		}
		for _, actionKey := range actionKeys {
			act, ok := v.GetMapStringAny("actions", actionKey)
			if !ok {
				delete(acts, actionKey)
			} else {
				acts[actionKey] = act
			}
		}
	}

	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	for _, act := range acts {
		switch act["op"] {
		case "copy":
			var ok bool
			var afrom, ato string
			if afrom, ok = act["from"].(string); !ok {
				continue
			}
			if ato, ok = act["to"].(string); !ok {
				continue
			}
			f, err := os.ReadFile(afrom)
			if err != nil {
				return err
			}
			err = os.WriteFile(path.Join(inst.ConfDir, ato), f, perm)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func geniMap(inst *instance) error {
	mapnames, ok := inst.cfg.GetKeys("maps")
	if !ok {
		return errors.New("no maps defined for preset")
	}
	if len(mapnames) == 0 {
		return errors.New("map list is empty")
	}
	mapn := rand.Intn(len(mapnames))
	inst.Settings.MapName = mapnames[mapn]
	inst.Settings.MapHash, ok = inst.cfg.GetString("maps", inst.Settings.MapName, "hash")
	if !ok {
		return errors.New("map hash not defined")
	}
	if !ok {
		return errors.New("map players not defined")
	}

	mapbytes, err := ms.GetMap(inst.Settings.MapHash)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(inst.ConfDir, "maps", inst.Settings.MapHash+".wz"), mapbytes, 0644)
}

func geniPreset(inst *instance) error {
	inst.Settings.TimeLimit = tryCfgGetD(tryGetIntGen("timelimit"), 2, inst.cfgs...)
	inst.Settings.PlayerCount = tryCfgGetD(tryGetIntGen("players"), -1, inst.cfgs...)
	if inst.Settings.PlayerCount < 2 {
		inst.logger.Println("Invalid playercount, aborting room creation!!!")
		return errors.New("invalid playercount")
	}
	inst.Settings.DisplayCategory = tryCfgGetD(tryGetIntGen("displayCategory"), 0, inst.cfgs...)
	inst.Settings.RatingCategories = tryCfgGetD(tryGetSliceIntGen("ratingCategories"), []int{}, inst.cfgs...)
	inst.BinPath = tryCfgGetD(tryGetStringGen("binary"), "warzone2100", inst.cfgs...)
	preset := map[string]any{
		"locked": map[string]any{
			"power":      true,
			"alliances":  false,
			"teams":      true,
			"difficulty": true,
			"ai":         true,
			"scavengers": false,
			"position":   false,
			"bases":      false,
		},
		"challenge": map[string]any{
			"map":                 inst.Settings.MapName,
			"maxPlayers":          inst.Settings.PlayerCount,
			"scavengers":          tryCfgGetD(tryPickNumberGen("settingsScavs"), 69, inst.cfgs...),
			"alliances":           tryCfgGetD(tryPickNumberGen("settingsAlliance"), 69, inst.cfgs...),
			"powerLevel":          tryCfgGetD(tryPickNumberGen("settingsPower"), 69, inst.cfgs...),
			"bases":               tryCfgGetD(tryPickNumberGen("settingsBase"), 69, inst.cfgs...),
			"name":                tryCfgGetD(tryGetStringGen("roomName"), "Welcome", inst.cfgs...),
			"techLevel":           tryCfgGetD(tryPickNumberGen("settingsTechLevel"), 1, inst.cfgs...),
			"spectatorHost":       true,
			"openSpectatorSlots":  tryCfgGetD(tryPickNumberGen("settingsSpecSlots"), 10, inst.cfgs...),
			"allowPositionChange": true,
		},
	}

	var presetOverride map[string]any
	for _, cfg := range inst.cfgs {
		o, ok := cfg.GetMapStringAny("presetOverride")
		if ok {
			presetOverride = o
			break
		}
	}

	if presetOverride == nil {
		for p := 0; p < inst.Settings.PlayerCount; p++ {
			var team int
			if inst.Settings.PlayerCount%2 != 0 {
				team = p
			} else {
				if p < inst.Settings.PlayerCount/2 {
					team = 0
				} else {
					team = 1
				}
			}
			preset[fmt.Sprintf("player_%d", p)] = map[string]any{
				"team": team,
			}
		}
	} else {
		for k, v := range presetOverride {
			preset[k] = v
		}
	}

	presetB, err := json.MarshalIndent(preset, "", "\t")
	if err != nil {
		return err
	}

	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	return os.WriteFile(path.Join(inst.ConfDir, "autohost", "preset.json"), presetB, perm)
}

func geniConfdir(inst *instance) string {
	return path.Join(cfg.GetDSString("./instances/", "instancesPath"), fmt.Sprint(inst.Id))
}

func geniAdminspolicy(inst *instance) ([]string, adminsPolicy) {
	switch tryCfgGetD(tryGetStringGen("adminsPolicy"), "", inst.cfgs...) {
	default:
		inst.logger.Println("Instance setting adminsPolicy is not declared *anywhere*, no admins for you!")
		fallthrough
	case "nobody":
		return []string{}, adminsPolicyNobody
	case "moderators":
		mods, err := fetchAdmins()
		if err != nil {
			inst.logger.Printf("Error fetching moderators: %s", err.Error())
			mods = []string{}
		}
		return mods, adminsPolicyModerators
	case "whitelist":
		admins := tryCfgGetD(tryGetSliceStringGen("admins"), nil, inst.cfgs...)
		if admins == nil {
			inst.logger.Printf("Instance setting admins for adminsPolicy is not declared *anywhere*, no admins for you!")
			mods, err := fetchAdmins()
			if err != nil {
				inst.logger.Printf("Error fetching moderators: %s", err.Error())
				mods = []string{}
			}
			return mods, adminsPolicyModerators
		}
		return admins, adminsPolicyWhitelist
	}
}

func fetchAdmins() ([]string, error) {
	ha := ""
	ret := []string{}
	_, err := dbpool.QueryFunc(context.Background(), `select
	hash
from identities
join accounts on identities.account = accounts.id
where accounts.allow_host_request = true and pkey is not null`, []any{}, []any{&ha}, func(qfr pgx.QueryFuncRow) error {
		ret = append(ret, ha)
		return nil
	})
	return ret, err
}

func geniBanlist(inst *instance) error {
	url := tryCfgGet(tryGetStringGen("fetchBanlist"), inst.cfgs...)
	if url == nil {
		inst.logger.Println("Instance setting fetchBanlist is not declared *anywhere*, no banlist for you!")
		return nil
	}
	cl := http.Client{
		Timeout: 5 * time.Second,
	}
	rsp, err := cl.Get(*url)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		return err
	}
	cl.CloseIdleConnections()
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	return os.WriteFile(path.Join(inst.ConfDir, "banlist.txt"), b, perm)
}
