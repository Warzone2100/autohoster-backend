package main

import (
	gamereport "autohoster-backend/gameReport"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/jackc/pgx/v4"
)

func submitReport(inst *instance, reportBytes []byte) {
	if inst.GameId <= 0 {
		inst.GameId = submitBegin(inst, reportBytes)
		err := recoverSave(inst)
		if err != nil {
			inst.logger.Printf("Failed to save instance recovery json: %s", err.Error())
			discordPostError("Failed to save instance recovery json: %s (instance %d)", err.Error(), inst.Id)
		}
	} else {
		submitFrame(inst, reportBytes)
	}
}

func submitFinalReport(inst *instance, reportBytes []byte) {
	if inst.GameId <= 0 {
		inst.logger.Printf("Trying to submit final report without valid game ID!")
	} else {
		submitEnd(inst, reportBytes)
	}
}

func submitBegin(inst *instance, reportBytes []byte) int {
	report := gamereport.GameReport{}
	err := json.Unmarshal(reportBytes, &report)
	if err != nil {
		inst.logger.Printf("Failed to unmarshal game report: %s report was %q", err.Error(), string(reportBytes))
		discordPostError("Failed to unmarshal game report: %s report was %q (instance %d)", err.Error(), string(reportBytes), inst.Id)
		return -1
	}
	var gid int
	ctx := context.Background()
	err = dbpool.BeginFunc(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into games (version, instance,
		setting_scavs, setting_alliance, setting_power, setting_base,
		map_name, map_hash, mods, display_category) values ($1, $2,
		$3, $4, $5, $6,
		$7, $8, $9, $10) returning id`, report.Game.Version, inst.Id,
			report.Game.Scavengers, report.Game.AlliancesType, report.Game.PowerType, report.Game.BaseType,
			inst.Settings.MapName, inst.Settings.MapHash, inst.Settings.Mods, inst.Settings.DisplayCategory).Scan(&gid)
		if err != nil {
			return err
		}
		for _, v := range report.PlayerData {
			if v.PublicKey == "" {
				continue
			}
			PublicKeyBytes, err := base64.StdEncoding.DecodeString(v.PublicKey)
			if err != nil {
				return err
			}
			pid := -1
			err = tx.QueryRow(ctx, `insert into identities (name, pkey, hash) values
	($1, $2, encode(sha256($2), 'hex'))
	on conflict (hash) do update set name = $1, pkey = $2 returning id;`, v.Name, PublicKeyBytes).Scan(&pid)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `insert into players (game, identity, position, team, color, props) values
				($1, $2, $3, $4, $5, $6)`, gid, pid, v.Position, v.Team, v.Color, v.GameReportPlayerStatistics)
			if err != nil {
				return err
			}
		}
		for _, v := range inst.Settings.RatingCategories {
			_, err := tx.Exec(ctx, `insert into games_rating_categories (game, category) values ($1, $2)`, gid, v)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		inst.logger.Printf("Failed to begin game: %s (gid %d)", err.Error(), inst.GameId)
		discordPostError("Failed to begin game: %s (gid %d) (instance %d)", err.Error(), inst.GameId, inst.Id)
	}
	return gid
}

var (
	frameErrorSuspends = map[int64]string{}
)

func submitFrame(inst *instance, reportBytes []byte) {
	report := gamereport.GameReport{}
	err := json.Unmarshal(reportBytes, &report)
	if err != nil {
		inst.logger.Printf("Failed to unmarshal game report: %s (gid %d) report was %q", err.Error(), inst.GameId, string(reportBytes))
		discordPostError("Failed to unmarshal game report: %s report was %q (instance %d)", err.Error(), string(reportBytes), inst.Id)
		return
	}
	frame := gamereport.GameReportGraphFrame{
		GameTime:                  report.GameTime,
		Kills:                     make([]int, len(report.PlayerData)),
		Power:                     make([]int, len(report.PlayerData)),
		Score:                     make([]int, len(report.PlayerData)),
		Droids:                    make([]int, len(report.PlayerData)),
		DroidsBuilt:               make([]int, len(report.PlayerData)),
		DroidsLost:                make([]int, len(report.PlayerData)),
		Hp:                        make([]int, len(report.PlayerData)),
		Structs:                   make([]int, len(report.PlayerData)),
		StructuresBuilt:           make([]int, len(report.PlayerData)),
		StructuresLost:            make([]int, len(report.PlayerData)),
		StructureKills:            make([]int, len(report.PlayerData)),
		SummExp:                   make([]int, len(report.PlayerData)),
		OilRigs:                   make([]int, len(report.PlayerData)),
		ResearchComplete:          make([]int, len(report.PlayerData)),
		RecentPowerLost:           make([]int, len(report.PlayerData)),
		RecentPowerWon:            make([]int, len(report.PlayerData)),
		RecentResearchPerformance: make([]int, len(report.PlayerData)),
		RecentResearchPotential:   make([]int, len(report.PlayerData)),
		RecentDroidPowerLost:      make([]int, len(report.PlayerData)),
		RecentStructurePowerLost:  make([]int, len(report.PlayerData)),
	}
	for i, v := range report.PlayerData {
		if v.PublicKey == "" {
			continue
		}
		frame.Kills[i] = v.Kills
		frame.Power[i] = v.Power
		frame.Score[i] = v.Score
		frame.Droids[i] = v.Droids
		frame.DroidsBuilt[i] = v.DroidsBuilt
		frame.DroidsLost[i] = v.DroidsLost
		frame.Hp[i] = v.Hp
		frame.Structs[i] = v.Structs
		frame.StructuresBuilt[i] = v.StructuresBuilt
		frame.StructuresLost[i] = v.StructuresLost
		frame.StructureKills[i] = v.StructureKills
		frame.SummExp[i] = v.SummExp
		frame.OilRigs[i] = v.OilRigs
		frame.ResearchComplete[i] = v.ResearchComplete
		frame.RecentPowerLost[i] = v.RecentPowerLost
		frame.RecentPowerWon[i] = v.RecentPowerWon
		frame.RecentResearchPerformance[i] = v.RecentResearchPerformance
		frame.RecentResearchPotential[i] = v.RecentResearchPotential
		frame.RecentDroidPowerLost[i] = v.RecentDroidPowerLost
		frame.RecentStructurePowerLost[i] = v.RecentStructurePowerLost
	}
	tag, err := dbpool.Exec(context.Background(), `update games set graphs = coalesce(graphs, '[]'::json)::jsonb || $1::jsonb where id = $2`, frame, inst.GameId)
	if err != nil {
		inst.logger.Printf("Failed to add game frame: %s (gid %d)", err.Error(), inst.GameId)
		discordPostError("Failed to add game frame: %s (gid %d) (instance %d)", err.Error(), inst.GameId, inst.Id)
	}
	if !tag.Update() || tag.RowsAffected() != 1 {
		inst.logger.Printf("SUS tag while adding game: %s (gid %d)", tag, inst.GameId)
		errMsgToSend := fmt.Sprintf("SUS tag while adding game: %s (gid %d) (instance %d)", tag, inst.GameId, inst.Id)
		errMsg, ok := frameErrorSuspends[inst.Id]
		if !ok {
			discordPostError(errMsgToSend)
			frameErrorSuspends[inst.Id] = errMsgToSend
		} else {
			if errMsg != errMsgToSend {
				discordPostError(errMsgToSend)
				frameErrorSuspends[inst.Id] = errMsgToSend
			}
		}
	}
}

func submitEnd(inst *instance, reportBytes []byte) {
	submitFrame(inst, reportBytes)
	report := gamereport.GameReportExtended{}
	err := json.Unmarshal(reportBytes, &report)
	if err != nil {
		inst.logger.Printf("Failed to unmarshal game report: %s (gid %d) report was %q", err.Error(), inst.GameId, string(reportBytes))
		return
	}
	err = dbpool.BeginFunc(context.Background(), func(tx pgx.Tx) error {
		for _, v := range report.PlayerData {
			if v.PublicKey == "" {
				continue
			}
			_, err := dbpool.Exec(context.Background(), `update players set usertype = $1, props = $2 where game = $3 and position = $4`,
				v.Usertype, v.GameReportPlayerStatistics, inst.GameId, v.Position)
			if err != nil {
				inst.logger.Printf("Failed to finalize player at position %d: %s (gid %d)", v.Position, err.Error(), inst.GameId)
				return err
			}
		}
		_, err = dbpool.Exec(context.Background(), `update games set research_log = $1, time_ended = TO_TIMESTAMP($2::double precision / 1000), debug_triggered = $3, game_time = $4 where id = $5`,
			report.ResearchComplete, report.EndDate, inst.DebugTriggered, report.GameTime, inst.GameId)
		if err != nil {
			inst.logger.Printf("Failed to finalize game: %s (gid %d)", err.Error(), inst.GameId)
		}
		return err
	})
	if err != nil {
		inst.logger.Printf("Failed to finalize: %s (gid %d)", err.Error(), inst.GameId)
		discordPostError("Failed to finalize: %s (gid %d) (instance %d)", err.Error(), inst.GameId, inst.Id)
	}
}

func sendReplayToStorage(inst *instance) {
	replayPath, err := findReplay(inst)
	if err != nil {
		inst.logger.Printf("Failed to find replay: %s", err.Error())
		discordPostError("Failed to find replay: %s (instance %d)", err.Error(), inst.Id)
		return
	}
	err = copyReplayToStorage(inst, replayPath)
	if err != nil {
		inst.logger.Printf("Failed to copy replay: %s", err.Error())
		discordPostError("Failed to copy replay: %s (instance %d)", err.Error(), inst.Id)
	}
	err = saveReplayToDatabase(inst, replayPath)
	if err != nil {
		inst.logger.Printf("Failed to save replay to database: %s", err.Error())
		discordPostError("Failed to save replay to database: %s (instance %d)", err.Error(), inst.Id)
	}
}

func saveReplayToDatabase(inst *instance, p string) error {
	rplBytes, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	rplCompressed, err := zstd.CompressLevel(nil, rplBytes, zstd.BestCompression)
	if err != nil {
		return err
	}
	tag, err := dbpool.Exec(context.Background(), `update games set replay = $1 where id = $2`, rplCompressed, inst.GameId)
	if err != nil {
		return err
	}
	if !tag.Update() || tag.RowsAffected() != 1 {
		return fmt.Errorf("sus tag: %s", tag.String())
	}
	return nil
}

func copyReplayToStorage(inst *instance, p string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	storageDir := getStorageReplayDir(inst.GameId)
	err = os.MkdirAll(storageDir, fs.FileMode(cfg.GetDInt(755, "dirPerms")))
	if err != nil {
		return err
	}
	storageFilePath := path.Join(storageDir, getStorageReplayFilename(inst.GameId)+".wzrp.zst")
	bc, err := zstd.Compress(nil, b)
	if err != nil {
		return err
	}
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	return os.WriteFile(storageFilePath, bc, perm)
}

func findReplay(inst *instance) (string, error) {
	replaydir := path.Join(inst.ConfDir, "replay", "multiplay")
	files, err := os.ReadDir(replaydir)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".wzrp") {
			h, err := os.Open(replaydir + "/" + f.Name())
			if err != nil {
				return "", err
			}
			var header [4]byte
			n, err := io.ReadFull(h, header[:])
			if err != nil {
				return "", err
			}
			h.Close()
			if n == 4 && string(header[:]) == "WZrp" {
				return replaydir + "/" + f.Name(), nil
			}
		}
	}
	return "", errors.New("replay not found")
}

func getStorageReplayDir(gid int) string {
	ret := cfg.GetDSString("./replayStorage/", "replayStorage")
	if ret == "" {
		ret = "./replayStorage/"
	}
	if gid <= 0 {
		return ret
	}
	num := ""
	for _, v := range big.NewInt(int64(gid)).Text(32) {
		num = string(v) + num
	}
	for _, n := range num[0 : len(num)-1] {
		ret = path.Join(ret, string(n))
	}
	return ret
}

func getStorageReplayFilename(gid int) string {
	if gid < 0 {
		gid = -gid
	}
	num := ""
	for _, v := range big.NewInt(int64(gid)).Text(32) {
		num = string(v) + num
	}
	return string(num[len(num)-1:])
}
