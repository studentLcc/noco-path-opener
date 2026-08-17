//go:build !windows || consoledebug

package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func waitForExit(server *http.Server) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	log.Printf("console debug mode active; press Ctrl+C to exit")
	<-stop
	shutdownServer(server)
	return nil
}
