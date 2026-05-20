package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"noco-path-opener/internal/actions"
	"noco-path-opener/internal/config"
	"noco-path-opener/internal/gui"
	"noco-path-opener/internal/nocodb"
	"noco-path-opener/internal/openapi"
	"noco-path-opener/internal/tray"
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
	opener := winopen.Opener{}
	nocoClient := nocodb.NewClient(nocodb.Config{
		BaseURL: cfg.NocoDBURL,
		Token:   cfg.NocoDBToken,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})
	remoteNocoClient := nocodb.NewClient(nocodb.Config{
		BaseURL: cfg.RemoteNocoDBURL,
		Token:   cfg.RemoteNocoDBToken,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})
	flow := &actions.Flow{
		Runner:           actions.NewLimitedRunner(gui.NewRunner(), cfg.MaxGUIWindows),
		Opener:           opener,
		Updater:          nocoClient,
		LocalSyncClient:  nocoClient,
		RemoteSyncClient: remoteNocoClient,
		AllowedRoots:     cfg.AllowedRoots,
		NocoDBURL:        cfg.NocoDBURL,
		NocoDBToken:      cfg.NocoDBToken,
		SyncProfiles:     syncProfilesFromConfig(cfg.SyncProfiles),
	}
	dispatcher := actions.NewAsyncDispatcher(flow, log.Default())

	server := &http.Server{
		Addr:    addr,
		Handler: openapi.NewServerWithWebhook(opener, cfg.AllowedRoots, dispatcher),
	}

	log.Printf("noco-path-opener listening on http://%s/open and http://%s/webhook", addr, addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	if err := tray.Run(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}); err != nil {
		log.Fatalf("tray failed: %v", err)
	}
}

func syncProfilesFromConfig(profiles []config.SyncProfile) []actions.SyncProfile {
	syncProfiles := make([]actions.SyncProfile, len(profiles))
	for i, profile := range profiles {
		syncProfiles[i] = actions.SyncProfile{
			Name:              profile.Name,
			LocalBaseID:       profile.LocalBaseID,
			LocalTableID:      profile.LocalTableID,
			LocalLookupField:  profile.LocalLookupField,
			RemoteBaseID:      profile.RemoteBaseID,
			RemoteTableID:     profile.RemoteTableID,
			RemoteLookupField: profile.RemoteLookupField,
			SyncFields:        append([]string(nil), profile.SyncFields...),
		}
	}
	return syncProfiles
}
