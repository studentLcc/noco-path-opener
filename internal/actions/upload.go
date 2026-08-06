package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"noco-path-opener/internal/nocodb"
)

func (c *flowController) UploadSelected(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return ErrPathRequired
	}
	if strings.TrimSpace(c.flow.NocoDBURL) == "" || strings.TrimSpace(c.flow.NocoDBToken) == "" {
		return ErrNocoDBConfigRequired
	}
	if c.flow.Updater == nil {
		return errors.New("updater is not configured")
	}

	sources := make([]string, 0, len(paths))
	for _, path := range paths {
		prepared, err := c.PreparePath(path)
		if err != nil {
			return err
		}
		sources = append(sources, prepared)
	}

	destination, destinationExists, err := c.uploadDestination()
	if err != nil {
		return err
	}
	if err := preflightUpload(destination, destinationExists, sources); err != nil {
		return err
	}

	if err := os.Mkdir(destination, 0o755); err != nil && !destinationExists {
		return fmt.Errorf("create upload destination: %w", err)
	}
	for _, source := range sources {
		if err := copyUploadPath(ctx, filepath.Join(destination, filepath.Base(source)), source); err != nil {
			return err
		}
	}

	if err := c.flow.Updater.UpdateRecord(ctx, nocodb.UpdateRequest{
		BaseID:    c.req.BaseID,
		TableID:   c.req.TableID,
		RecordID:  c.req.RecordID,
		PathField: c.req.PathField,
		PathValue: destination,
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrUploadWriteBackFailed, err)
	}

	c.req.CurrentPath = destination
	return nil
}

func (c *flowController) uploadDestination() (string, bool, error) {
	current := strings.TrimSpace(c.req.CurrentPath)
	if current != "" {
		prepared, err := c.PreparePath(current)
		if err != nil {
			return "", false, err
		}
		info, err := os.Stat(prepared)
		if err != nil {
			return "", false, err
		}
		if !info.IsDir() {
			return "", false, ErrUploadDestinationNotDirectory
		}
		return prepared, true, nil
	}

	baseDir := strings.TrimSpace(c.req.BaseDir)
	if baseDir == "" {
		return "", false, errors.New("base_dir is required for upload")
	}
	folderName := strings.TrimSpace(c.req.FolderName)
	if !validUploadFolderName(folderName) {
		return "", false, ErrUploadFolderNameInvalid
	}
	base, err := c.PreparePath(baseDir)
	if err != nil {
		return "", false, err
	}
	baseInfo, err := os.Stat(base)
	if err != nil {
		return "", false, err
	}
	if !baseInfo.IsDir() {
		return "", false, ErrUploadDestinationNotDirectory
	}

	destination := filepath.Join(base, folderName)
	allowed, err := isAllowed(destination, c.flow.AllowedRoots)
	if err != nil {
		return "", false, err
	}
	if !allowed {
		return "", false, ErrPathNotAllowed
	}
	if _, err := os.Stat(destination); err == nil {
		return "", false, ErrUploadDestinationExists
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	return destination, false, nil
}

func validUploadFolderName(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return false
	}
	return !strings.ContainsAny(name, `/\:*?"<>|`)
}

func preflightUpload(destination string, destinationExists bool, sources []string) error {
	names := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		name := filepath.Base(source)
		key := strings.ToLower(name)
		if _, exists := names[key]; exists {
			return fmt.Errorf("upload source name conflicts: %s", name)
		}
		names[key] = struct{}{}
		if destinationExists {
			if _, err := os.Lstat(filepath.Join(destination, name)); err == nil {
				return fmt.Errorf("upload target already exists: %s", name)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.IsDir() && isSameOrChildPath(destination, source) {
			return ErrUploadSourceConflict
		}
	}
	return nil
}

func isSameOrChildPath(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if samePath(path, root) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func copyUploadPath(ctx context.Context, destination, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyUploadDirectory(ctx, destination, source)
	}
	return copyUploadFile(ctx, destination, source, info.Mode())
}

func copyUploadDirectory(ctx context.Context, destination, source string) error {
	if err := os.Mkdir(destination, 0o755); err != nil {
		return fmt.Errorf("create upload directory %q: %w", destination, err)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyUploadFile(ctx, target, path, info.Mode())
	})
}

func copyUploadFile(ctx context.Context, destination, source string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return fmt.Errorf("create upload file %q: %w", destination, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}
