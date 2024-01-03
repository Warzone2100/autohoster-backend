package main

import (
	"log"

	"github.com/maxsupermanhd/lac"
)

var (
	cfg *lac.Conf
)

func main() {
	log.Println("Hello world")
	loadConfig()
}
