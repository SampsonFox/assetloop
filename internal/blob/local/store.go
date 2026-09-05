package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

type Store struct{ root string }

func New(root string) (*Store, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local blob root is required")
	}
	return &Store{root: filepath.Clean(abs)}, nil
}

func (s *Store) path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", errors.New("invalid object key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid object key")
	}
	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("object key escapes root")
	}
	return path, nil
}

func (s *Store) Put(ctx context.Context, key string, src io.Reader, _ application.BlobMetadata) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".blob-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = io.Copy(tmp, &contextReader{ctx: ctx, r: src}); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if existing, statErr := os.Stat(path); statErr == nil {
		written, statErr := os.Stat(name)
		if statErr == nil && existing.Size() == written.Size() {
			return nil
		}
		return errors.New("blob already exists with a different size")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	return os.Rename(name, path)
}

func (s *Store) Open(_ context.Context, key string) (io.ReadCloser, application.BlobInfo, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, application.BlobInfo{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, application.BlobInfo{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, application.BlobInfo{}, err
	}
	return file, application.BlobInfo{Size: info.Size()}, nil
}

func (s *Store) Stat(_ context.Context, key string) (application.BlobInfo, error) {
	path, err := s.path(key)
	if err != nil {
		return application.BlobInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return application.BlobInfo{}, err
	}
	return application.BlobInfo{Size: info.Size()}, nil
}
func (s *Store) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}
