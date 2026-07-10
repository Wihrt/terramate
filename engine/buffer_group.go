// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package engine

import (
	"bytes"
	"io"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/terramate-io/terramate/errors"
)

// BufferGroup manages a group of synchronized buffers so that concurrent
// writers (e.g. parallel stack runs) don't interleave mid-line.
type BufferGroup struct {
	fds []io.Closer
	wg  sync.WaitGroup
}

// NewBufferGroup creates a new buffer group.
func NewBufferGroup() *BufferGroup {
	return &BufferGroup{}
}

// Wait waits for the processing of all output for the buffers within this group.
// After calling this method, it's not safe to call any other method, as it
// closes the internal channels and shuts down all goroutines.
func (s *BufferGroup) Wait() {
	for _, writerFD := range s.fds {
		// only returns an error when readerFD.CloseWithError(err) is called,
		// but this is not the case.
		_ = writerFD.Close()
	}
	s.wg.Wait()
}

// NewBuffer creates a new synchronized, line-buffered writer.
func (s *BufferGroup) NewBuffer(out io.Writer) io.Writer {
	r, w := io.Pipe()
	s.fds = append(s.fds, w)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var pending []byte
		errs := errors.L()
		for {
			lines, rest, readErr := readLines(r, pending)
			if readErr != nil && readErr != io.EOF {
				errs.Append(readErr)
				break
			}
			if readErr == io.EOF && len(rest) > 0 {
				lines = [][]byte{rest}
			}
			for _, line := range lines {
				_, err := out.Write(line)
				if err != nil {
					errs.Append(errors.E(err, "writing to terminal"))
				}
			}
			if readErr == io.EOF {
				break
			}
			pending = rest
		}

		errs.Append(r.Close())
		errs.Append(w.Close())
		if err := errs.AsError(); err != nil {
			log.Error().Err(err).Msg("synchronizing command output")
		}
	}()
	return w
}

func readLines(r io.Reader, pending []byte) (line [][]byte, rest []byte, err error) {
	const readSize = 1024

	var buf [readSize]byte
	rest = pending
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			rest = append(rest, buf[:n]...)
			var lines [][]byte

			var nlpos int
			for nlpos != -1 {
				nlpos = bytes.IndexByte(rest, '\n')
				if nlpos >= 0 {
					lines = append(lines, rest[:nlpos+1]) // line includes ln
					rest = rest[nlpos+1:]
				}
			}
			if len(lines) > 0 {
				return lines, rest, err
			}
		} else if err == nil {
			// misbehaving reader
			return nil, nil, io.EOF
		}
		if err != nil {
			return nil, rest, err
		}

		// line ending not found, continue reading.
	}
}
