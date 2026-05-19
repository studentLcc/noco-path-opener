//go:build windows

package tray

import (
	"runtime"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

const toolTip = "Noco Path Opener"

func Run(onExit func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mw, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer mw.Dispose()

	notifyIcon, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return err
	}
	defer notifyIcon.Dispose()

	icon, err := defaultIcon()
	if err != nil {
		return err
	}
	if err := notifyIcon.SetIcon(icon); err != nil {
		return err
	}
	if err := notifyIcon.SetToolTip(toolTip); err != nil {
		return err
	}

	exitAction := walk.NewAction()
	if err := exitAction.SetText("退出"); err != nil {
		return err
	}
	exitAction.Triggered().Attach(func() {
		if onExit != nil {
			onExit()
		}
		walk.App().Exit(0)
	})
	if err := notifyIcon.ContextMenu().Actions().Add(exitAction); err != nil {
		return err
	}

	if err := notifyIcon.SetVisible(true); err != nil {
		return err
	}

	mw.Run()
	return nil
}

func defaultIcon() (*walk.Icon, error) {
	hIcon := win.LoadIcon(0, win.MAKEINTRESOURCE(win.IDI_APPLICATION))
	return walk.NewIconFromHICON(hIcon)
}
