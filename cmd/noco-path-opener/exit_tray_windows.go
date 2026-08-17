//go:build windows && !consoledebug

package main

import (
	"net/http"

	"noco-path-opener/internal/tray"
)

func waitForExit(server *http.Server) error {
	return tray.Run(func() {
		shutdownServer(server)
	})
}
