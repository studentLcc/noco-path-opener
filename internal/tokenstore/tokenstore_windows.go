//go:build windows

package tokenstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cryptProtectUIForbidden = 0x1

var (
	crypt32                = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
)

type dataBlob struct {
	size uint32
	data *byte
}

func (s *Store) Load() (string, error) {
	encrypted, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read saved snc-token: %w", err)
	}
	if len(encrypted) == 0 {
		return "", errors.New("saved snc-token is empty")
	}

	decrypted, err := unprotect(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt saved snc-token: %w", err)
	}
	return string(decrypted), nil
}

func (s *Store) Save(token string) error {
	if token == "" {
		return errors.New("snc-token is empty")
	}

	encrypted, err := protect([]byte(token))
	if err != nil {
		return fmt.Errorf("encrypt snc-token: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create snc-token directory: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, tokenFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary snc-token file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if err := tempFile.Chmod(0o600); err != nil {
		tempFile.Close()
		return fmt.Errorf("secure temporary snc-token file: %w", err)
	}
	if _, err := tempFile.Write(encrypted); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temporary snc-token file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("flush temporary snc-token file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary snc-token file: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("replace saved snc-token: %w", err)
	}
	return nil
}

func protect(plain []byte) ([]byte, error) {
	input := blobFromBytes(plain)
	var output dataBlob

	result, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0,
		0,
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&output)),
	)
	runtime.KeepAlive(plain)
	if result == 0 {
		return nil, callError(callErr)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.data))))

	return bytesFromBlob(output)
}

func unprotect(encrypted []byte) ([]byte, error) {
	input := blobFromBytes(encrypted)
	var output dataBlob

	result, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0,
		0,
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&output)),
	)
	runtime.KeepAlive(encrypted)
	if result == 0 {
		return nil, callError(callErr)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.data))))

	return bytesFromBlob(output)
}

func blobFromBytes(data []byte) dataBlob {
	return dataBlob{
		size: uint32(len(data)),
		data: &data[0],
	}
}

func bytesFromBlob(blob dataBlob) ([]byte, error) {
	if blob.size == 0 {
		return []byte{}, nil
	}
	if blob.data == nil {
		return nil, errors.New("DPAPI returned an invalid data buffer")
	}
	return append([]byte(nil), unsafe.Slice(blob.data, blob.size)...), nil
}

func callError(err error) error {
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return errors.New("DPAPI call failed")
	}
	return err
}
