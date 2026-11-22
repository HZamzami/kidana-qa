// Package storage keeps uploaded files (inspection photos). The Store
// interface lets a future S3/MinIO implementation replace the local disk.
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store saves and opens files by key.
type Store interface {
	Save(ctx context.Context, r io.Reader) (key string, size int64, contentType string, err error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key string) error
}

// AllowedImageTypes are the content types accepted for photos (sniffed, not trusted from the client).
var AllowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// ErrUnsupportedType is returned when the sniffed type is not an allowed image.
var ErrUnsupportedType = errors.New("unsupported file type")

// Disk stores files under a root directory, sharded by year/month.
type Disk struct {
	Root     string
	MaxBytes int64
}

func NewDisk(root string, maxBytes int64) (*Disk, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	return &Disk{Root: root, MaxBytes: maxBytes}, nil
}

func (d *Disk) Save(_ context.Context, r io.Reader) (string, int64, string, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", 0, "", err
	}
	head = head[:n]
	ctype := http.DetectContentType(head)
	ext, ok := AllowedImageTypes[ctype]
	if !ok {
		return "", 0, "", ErrUnsupportedType
	}

	key, err := newKey(ext)
	if err != nil {
		return "", 0, "", err
	}
	path := filepath.Join(d.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", 0, "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640) //nolint:gosec // path built from random key
	if err != nil {
		return "", 0, "", err
	}
	limited := io.LimitReader(io.MultiReader(bytes.NewReader(head), r), d.MaxBytes+1)
	size, copyErr := io.Copy(f, limited)
	closeErr := f.Close()
	if copyErr == nil && size > d.MaxBytes {
		copyErr = fmt.Errorf("file exceeds %d MB", d.MaxBytes>>20)
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(path)
		return "", 0, "", err
	}
	return key, size, ctype, nil
}

func (d *Disk) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	path, err := d.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path) //nolint:gosec // resolved inside root
}

func (d *Disk) Delete(_ context.Context, key string) error {
	path, err := d.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// resolve maps a key to a path and refuses anything that escapes Root.
func (d *Disk) resolve(key string) (string, error) {
	root, err := filepath.Abs(d.Root)
	if err != nil {
		return "", err
	}
	p := filepath.Join(root, filepath.FromSlash(key))
	if rel, err := filepath.Rel(root, p); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid storage key")
	}
	return p, nil
}

func newKey(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf("%04d/%02d/%s%s", now.Year(), now.Month(), hex.EncodeToString(b), ext), nil
}
