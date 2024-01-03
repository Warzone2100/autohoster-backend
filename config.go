package main

import (
	"log"

	"github.com/maxsupermanhd/lac"
)

func loadConfig() {
	var err error
	cfg, err = lac.FromFileJSON("config.json")
	if err != nil {
		log.Fatalf("Failed to read config: %s", err.Error())
	}
}
