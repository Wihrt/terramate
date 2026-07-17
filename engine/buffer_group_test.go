// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package engine

import (
	"bytes"
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
