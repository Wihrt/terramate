// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package engine

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestBufferGroupWritesCompleteLines(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w1 := bg.NewBuffer(safeOut)
	w2 := bg.NewBuffer(safeOut)

	_, err := w1.Write([]byte("hello from w1\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w2.Write([]byte("hello from w2\n"))
	if err != nil {
		t.Fatal(err)
	}

	bg.Wait()

	got := out.String()
	if got != "hello from w1\nhello from w2\n" && got != "hello from w2\nhello from w1\n" {
		t.Fatalf("expected two complete, non-interleaved lines, got: %q", got)
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestBufferGroupReassemblesPartialWrites(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w := bg.NewBuffer(safeOut)

	for _, chunk := range []string{"hel", "lo\nwor", "ld\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	bg.Wait()

	if got := out.String(); got != "hello\nworld\n" {
		t.Fatalf("expected reassembled lines, got: %q", got)
	}
}

func TestBufferGroupFlushesTrailingDataWithoutNewline(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w := bg.NewBuffer(safeOut)

	if _, err := w.Write([]byte("no trailing newline")); err != nil {
		t.Fatal(err)
	}
	bg.Wait()

	if got := out.String(); got != "no trailing newline" {
		t.Fatalf("expected trailing data flushed on EOF, got: %q", got)
	}
}

func TestBufferGroupInterleavedPartialWritesKeepLinesIntact(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w1 := bg.NewBuffer(safeOut)
	w2 := bg.NewBuffer(safeOut)

	writes := []struct {
		w     io.Writer
		chunk string
	}{
		{w1, "a"}, {w2, "x"}, {w1, "b\n"}, {w2, "y\n"},
	}
	for _, wr := range writes {
		if _, err := wr.w.Write([]byte(wr.chunk)); err != nil {
			t.Fatal(err)
		}
	}
	bg.Wait()

	got := out.String()
	if got != "ab\nxy\n" && got != "xy\nab\n" {
		t.Fatalf("expected complete lines ab/xy in either order, got: %q", got)
	}
}

// failingWriter fails every write, exercising the error-logging path of the
// buffer goroutine.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("sink failed")
}

func TestBufferGroupWriteErrorDoesNotDeadlock(t *testing.T) {
	t.Parallel()

	bg := NewBufferGroup()
	w := bg.NewBuffer(failingWriter{})

	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatal(err)
	}
	// Wait must return despite the sink error (which is only logged).
	bg.Wait()
}

// misbehavingReader returns (0, nil), which readLines must treat as EOF
// instead of spinning forever.
type misbehavingReader struct{}

func (misbehavingReader) Read([]byte) (int, error) { return 0, nil }

func TestReadLinesMisbehavingReaderReturnsEOF(t *testing.T) {
	t.Parallel()

	lines, rest, err := readLines(misbehavingReader{}, nil)
	if err != io.EOF {
		t.Fatalf("expected io.EOF for misbehaving reader, got: %v", err)
	}
	if lines != nil || rest != nil {
		t.Fatalf("expected no data, got lines=%v rest=%q", lines, rest)
	}
}

func TestReadLinesKeepsPendingAcrossCalls(t *testing.T) {
	t.Parallel()

	lines, rest, err := readLines(strings.NewReader("tail"), []byte("head-"))
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no complete line, got: %q", lines)
	}
	if string(rest) != "head-tail" {
		t.Fatalf("expected pending bytes preserved, got: %q", rest)
	}
}
