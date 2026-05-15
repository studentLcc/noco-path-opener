//go:build !windows

package winopen

import "fmt"

type Opener struct{}

func (Opener) Open(path string, isDir bool) error {
	return fmt.Errorf("opening paths is only supported on Windows")
}
