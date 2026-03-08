package main

import (
	"log"

	"anonvote/internal/config"
	"anonvote/internal/di"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	server, err := di.InitializeServer(cfg)
	if err != nil {
		log.Fatalf("initialize server: %v", err)
	}

	if err := server.Run(); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
