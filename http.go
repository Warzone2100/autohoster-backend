package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/maxsupermanhd/lac/v2"
)

func routineWebServer(closechan <-chan struct{}) {
	m := http.NewServeMux()
	m.HandleFunc("/instances", webHandleInstances)
	m.HandleFunc("/reload", webHandleReload)
	m.HandleFunc("/alive", webHandleAlive)
	m.HandleFunc("/request", webHandleRequestRoom)
	var wg sync.WaitGroup
	wg.Add(1)
	srv := http.Server{
		Addr:              cfg.GetDSString("127.0.0.1:9271", "listenAddr"),
		Handler:           m,
		ReadTimeout:       time.Second * 2,
		ReadHeaderTimeout: time.Second * 2,
		WriteTimeout:      time.Second * 2,
		IdleTimeout:       time.Second * 2,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Printf("HTTP server error: %s", err)
			}
		}
		wg.Done()
	}()
	<-closechan
	log.Println("Shutting down http server...")
	srv.Close()
	log.Println("Waiting for http server to exit...")
	wg.Wait()
}

func webHandleInstances(w http.ResponseWriter, r *http.Request) {
	ret := map[int64]any{}
	instancesLock.Lock()
	for _, v := range instances {
		cfgs := []any{}
		for _, c := range v.cfgs {
			a, _ := c.Get()
			cfgs = append(cfgs, a)
		}
		inst := map[string]any{
			"state":    v.state.Load(),
			"pid":      v.Pid,
			"game id":  v.GameId,
			"lobby id": v.LobbyId,
			"settings": v.Settings,
			"cfgs":     cfgs,
		}
		ret[v.Id] = inst
	}
	instancesLock.Unlock()
	b, err := json.MarshalIndent(ret, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		w.Write([]byte("\n"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(b)
	w.Write([]byte("\n"))
}

func webHandleReload(w http.ResponseWriter, r *http.Request) {
	err := cfg.SetFromFileJSON("config.json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Config reloaded"))
}

func webHandleAlive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Autohoster backend online, room creation allowed: " + fmt.Sprint(!disallowInstanceCreation.Load())))
}

func webHandleRequestRoom(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("Failed to read body: %s", err.Error())
		return
	}
	c, err := lac.FromBytesJSON(b)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("Failed to unmarshal json: %s", err.Error())
		return
	}
	gi, err := generateInstance(c)
	if err != nil {
		log.Printf("Failed to generate instance: %s", err.Error())
		if gi != nil {
			releaseInstance(gi)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	gi.QueueName = ""
	go spawnRunner(gi)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Room created, join with host.wz2100-autohost.net:%d", gi.Settings.GamePort)))
}
