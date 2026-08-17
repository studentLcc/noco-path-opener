//go:build !windows

package tokenstore

import "errors"

func (s *Store) Load() (string, error) {
	return "", nil
}

func (s *Store) Save(token string) error {
	return errors.New("snc-token persistence is only supported on Windows")
}
