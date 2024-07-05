package main

import (
	"archive/tar"
	gamereport "autohoster-backend/gameReport"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime/debug"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	archiveDirPath = flag.String("archivesDir", "./run/archive/", "path to directory with archives")
	gid            = flag.Int("gid", -1, "game id")
	instanceId     = flag.Int64("instance", -1, "instance id")
	connString     = flag.String("connString", "", "database connection string")
)

func main() {
	archiveId := archiveInstanceIdToWeek(*instanceId)
	log.Printf("Looking for instance %d in archive %d", *instanceId, archiveId)
	archiveNameNormal := fmt.Sprintf("%d.tar", archiveId)
	archivePathNormal := path.Join(*archiveDirPath, archiveNameNormal)
	archiveNameCompressed := fmt.Sprintf("%d.tar.zst", archiveId)
	archivePathCompressed := path.Join(*archiveDirPath, archiveNameCompressed)
	st, err := os.Stat(archivePathNormal)
	if err == nil && !st.IsDir() {
		log.Printf("Found %q", archivePathNormal)
		processArchive(archivePathNormal)
		return
	}
	st, err = os.Stat(archivePathCompressed)
	if err == nil && !st.IsDir() {
		log.Printf("Found %q", archivePathCompressed)
		processCompressedArchive(archivePathCompressed)
		return
	}
}

func submitGameEnd(report gamereport.GameReportExtended) {
	dbpool := noerr(pgxpool.Connect(context.Background(), *connString))
	err := dbpool.BeginFunc(context.Background(), func(tx pgx.Tx) error {
		for _, v := range report.PlayerData {
			_, err := dbpool.Exec(context.Background(), `update players set usertype = $1, props = $2 where game = $3 and position = $4`,
				v.Usertype, v.GameReportPlayerStatistics, *gid, v.Position)
			if err != nil {
				log.Printf("Failed to finalize player at position %d: %s (gid %d)", v.Position, err.Error(), *gid)
				return err
			}
		}
		_, err := dbpool.Exec(context.Background(), `update games set research_log = $1, time_ended = TO_TIMESTAMP($2::double precision / 1000), game_time = $3 where id = $4`,
			report.ResearchComplete, report.EndDate, report.GameTime, *gid)
		if err != nil {
			log.Printf("Failed to finalize game: %s (gid %d)", err.Error(), *gid)
		}
		return err
	})
	if err != nil {
		log.Printf("Failed to finalize: %s (gid %d)", err.Error(), *gid)
	}
}

func processGameLog(g string) {
	gs := strings.Split(g, "\n")
	for i, v := range gs {
		if strings.HasPrefix(v, "__REPORTextended__") && strings.HasSuffix(v, "__ENDREPORTextended__") {
			v = strings.TrimPrefix(v, "__REPORTextended__")
			v = strings.TrimSuffix(v, "__ENDREPORTextended__")
			rpt := gamereport.GameReportExtended{}
			log.Printf("Extended report found at line %d, len %d", i, len(v))
			must(json.Unmarshal([]byte(v), &rpt))
			submitGameEnd(rpt)
		}
	}
}

func processTar(f io.Reader) {
	r := tar.NewReader(f)
	for {
		h := noerr(r.Next())
		fb := path.Base(h.Name)
		if !strings.HasPrefix(fb, "gamelog_") {
			continue
		}
		if !strings.HasSuffix(fb, ".log") {
			continue
		}
		buf := make([]byte, h.Size)
		i := noerr(r.Read(buf))
		if i != int(h.Size) {
			log.Printf("tar read i %d != size %d", i, h.Size)
		}
		processGameLog(string(buf))
	}
}

func processArchive(p string) {
	f := noerr(os.Open(p))
	processTar(f)
	f.Close()
}

func processCompressedArchive(_ string) {
	log.Println("not implemented compressed archive scanning")
	// TODO
}

func archiveInstanceIdToWeek(num int64) int64 {
	return num / (7 * 24 * 60 * 60)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
		debug.PrintStack()
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
}
