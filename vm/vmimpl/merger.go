// Copyright 2016 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package vmimpl

import (
	"bytes"
	"fmt"
	"io"
	"sync"
)

type OutputMerger struct {
	Output chan []byte
	Err    chan error
	teeMu  sync.Mutex
	tee    io.Writer
	wg     sync.WaitGroup
}

func NewOutputMerger(tee io.Writer) *OutputMerger {
	return &OutputMerger{
		Output: make(chan []byte, 1000),
		Err:    make(chan error, 1),
		tee:    tee,
	}
}

func (merger *OutputMerger) Wait() {
	merger.wg.Wait()
	close(merger.Output)
}

func (merger *OutputMerger) Add(name string, r io.ReadCloser) {
	merger.wg.Add(1)
	go func() {
		var pending []byte
		var buf [4 << 10]byte
		for {
			n, err := r.Read(buf[:])
			if n != 0 {
				pending = append(pending, buf[:n]...)
				if pos := bytes.LastIndexByte(pending, '\n'); pos != -1 {
					out := pending[:pos+1]
					if merger.tee != nil {
						merger.teeMu.Lock()
						merger.tee.Write(out)
						merger.teeMu.Unlock()
					}
					select {
					case merger.Output <- append([]byte{}, out...):
						r := copy(pending[:], pending[pos+1:])
						pending = pending[:r]
					default:
					}
				}
			}
			if err != nil {
				if len(pending) != 0 {
					pending = append(pending, '\n')
					if merger.tee != nil {
						merger.teeMu.Lock()
						merger.tee.Write(pending)
						merger.teeMu.Unlock()
					}
					select {
					case merger.Output <- pending:
					default:
					}
				}
				r.Close()
				select {
				case merger.Err <- fmt.Errorf("failed to read from %v: %v", name, err):
				default:
				}
				merger.wg.Done()
				return
			}
		}
	}()
}
