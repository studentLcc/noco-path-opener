//go:build windows

package winopen

import "os/exec"

type Opener struct{}

func (Opener) Open(path string, isDir bool) error {
	if isDir {
		return exec.Command("explorer.exe", path).Start()
	}
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Start()
}
