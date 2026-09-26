// Package storage stores binary blobs (attachments, logos) by key.
// Local keeps them on disk; an S3-compatible implementation can be added
// behind the same interface.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// ErrNotFound is returned by Get and Delete for an unknown key.
var ErrNotFound = errors.New("storage: not found")

// Storage stores blobs by key. Keys are slash-separated relative paths of
// [A-Za-z0-9._-] segments (no "." / ".." segments), e.g. "attachments/12/3f9a…".
type Storage interface {
	// Put stores r under key, replacing any existing blob.
	Put(ctx context.Context, key string, r io.Reader) error
	// Get opens the blob; the caller closes it. Unknown key → ErrNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the blob. Unknown key → ErrNotFound.
	Delete(ctx context.Context, key string) error
}

var keyRe = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*(/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$`)

// ValidKey reports whether key is acceptable for every Storage implementation.
func ValidKey(key string) bool { return len(key) <= 512 && keyRe.MatchString(key) }

// Local stores blobs as files under a directory (created on first Put).
type Local struct {
	dir string
}

func NewLocal(dir string) *Local { return &Local{dir: dir} }

func (l *Local) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}
	return filepath.Join(l.dir, filepath.FromSlash(key)), nil
}

func (l *Local) Put(ctx context.Context, key string, r io.Reader) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := io.Copy(tmp, readerCtx{ctx, r}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return nil
}

// readerCtx stops copying when ctx is cancelled.
type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r readerCtx) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
