package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// 1x1 PNG
var png1x1 = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4, 0, 0, 0, 0, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

func TestDiskRoundTrip(t *testing.T) {
	d, err := NewDisk(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key, size, ctype, err := d.Save(ctx, bytes.NewReader(png1x1))
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "image/png" || size != int64(len(png1x1)) {
		t.Fatalf("got %s %d", ctype, size)
	}
	f, err := d.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if !bytes.Equal(got, png1x1) {
		t.Fatal("content mismatch")
	}
	if err := d.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
}

func TestDiskRejects(t *testing.T) {
	d, _ := NewDisk(t.TempDir(), 32)
	ctx := context.Background()
	if _, _, _, err := d.Save(ctx, bytes.NewReader([]byte("<html>not an image</html>"))); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("want ErrUnsupportedType, got %v", err)
	}
	if _, _, _, err := d.Save(ctx, bytes.NewReader(png1x1)); err == nil {
		t.Fatal("expected size limit error")
	}
	if _, err := d.Open(ctx, "../../etc/passwd"); err == nil {
		t.Fatal("path traversal not rejected")
	}
}
