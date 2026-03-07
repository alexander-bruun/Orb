package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalFS stores objects on the local filesystem under a root directory.
type LocalFS struct {
	root string
}

// NewLocalFS returns a LocalFS backed by root. The directory is created if needed.
func NewLocalFS(root string) (*LocalFS, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create store root %q: %w", root, err)
	}
	return &LocalFS{root: root}, nil
}

func (l *LocalFS) path(key string) string {
	return filepath.Join(l.root, filepath.FromSlash(key))
}

func (l *LocalFS) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	dest := l.path(key)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	return nil
}

func (l *LocalFS) GetRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	f, err := os.Open(l.path(key))
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", key, err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, fmt.Errorf("seek %q: %w", key, err)
	}
	return &limitedReadCloser{r: io.LimitReader(f, length), c: f}, nil
}

func (l *LocalFS) Delete(_ context.Context, key string) error {
	err := os.Remove(l.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (l *LocalFS) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(l.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (l *LocalFS) Size(_ context.Context, key string) (int64, error) {
	fi, err := os.Stat(l.path(key))
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

type limitedReadCloser struct {
	r io.Reader
	c io.Closer
}

func (l *limitedReadCloser) Read(p []byte) (int, error) { return l.r.Read(p) }
func (l *limitedReadCloser) Close() error               { return l.c.Close() }
