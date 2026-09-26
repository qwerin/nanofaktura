package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocal(t *testing.T) {
	ctx := context.Background()
	s := NewLocal(t.TempDir() + "/nested")

	if err := s.Put(ctx, "attachments/1/abc.pdf", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "attachments/1/abc.pdf", strings.NewReader("second")); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, "attachments/1/abc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "second" {
		t.Fatalf("content %q", b)
	}
	if err := s.Delete(ctx, "attachments/1/abc.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "attachments/1/abc.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := s.Delete(ctx, "attachments/1/abc.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}

	for _, bad := range []string{"", "../x", "a/../../x", "/abs", "a//b", "a/./b", ".hidden", "a\\b"} {
		if err := s.Put(ctx, bad, strings.NewReader("x")); err == nil {
			t.Fatalf("key %q accepted", bad)
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Put(cctx, "x", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put: %v", err)
	}
	if _, err := s.Get(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled put left a file: %v", err)
	}
}
