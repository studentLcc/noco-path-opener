package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"noco-path-opener/internal/config"
	"noco-path-opener/internal/openapi"
	"noco-path-opener/internal/winopen"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("find executable path: %v", err)
	}

	configPath := filepath.Join(filepath.Dir(exePath), "config.json")
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: openapi.NewServer(winopen.Opener{}, cfg.AllowedRoots),
	}

	log.Printf("noco-path-opener listening on http://%s/open", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}
