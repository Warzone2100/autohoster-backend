package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	dbpool *pgxpool.Pool
)

func connectToDatabase() {
	connstr, ok := cfg.GetString("databaseConnString")
	if !ok {
		log.Fatalf("No databaseConnString found in config, unable to connect to database!")
	}
	var err error
	dbpool, err = pgxpool.Connect(context.Background(), connstr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %s", err.Error())
	}
}
