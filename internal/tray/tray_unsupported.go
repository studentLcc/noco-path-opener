//go:build !windows

package tray

import (
	"os"
	"os/signal"
	"syscall"
)

func Run(onExit func()) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	<-stop
	if onExit != nil {
		onExit()
	}
	return nil
}
