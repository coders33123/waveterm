// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// allows for streaming an io.Reader to a channel and an io.Writer from a channel
package iochan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/wavetermdev/waveterm/pkg/util/iochan/iochantypes"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// ReaderChan reads from an io.Reader and sends the data to a channel
func ReaderChan(ctx context.Context, r io.Reader, chunkSize int64, callback func()) chan wshrpc.RespOrErrorUnion[iochantypes.Packet] {
	ch := make(chan wshrpc.RespOrErrorUnion[iochantypes.Packet], 32)
	sha256Hash := sha256.New()
	go func() {
		defer func() {
			close(ch)
			callback()
		}()
		for {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.Canceled {
					return
				}
				return
			default:
				buf := make([]byte, chunkSize)
				if n, err := r.Read(buf); err != nil {
					if errors.Is(err, io.EOF) {
						ch <- wshrpc.RespOrErrorUnion[iochantypes.Packet]{Response: iochantypes.Packet{Checksum: sha256Hash.Sum(nil)}} // send the checksum
						return
					}
					ch <- wshutil.RespErr[iochantypes.Packet](fmt.Errorf("ReaderChan: read error: %v", err))
					return
				} else if n > 0 {
					if _, err := sha256Hash.Write(buf[:n]); err != nil {
						ch <- wshutil.RespErr[iochantypes.Packet](fmt.Errorf("ReaderChan: error writing to sha256 hash: %v", err))
						return
					}
					ch <- wshrpc.RespOrErrorUnion[iochantypes.Packet]{Response: iochantypes.Packet{Data: buf[:n]}}
				}
			}
		}
	}()
	return ch
}

// WriterChan reads from a channel and writes the data to an io.Writer
func WriterChan(ctx context.Context, w io.Writer, ch <-chan wshrpc.RespOrErrorUnion[iochantypes.Packet], callback func(), errCallback func(error)) {
	sha256Hash := sha256.New()
	go func() {
		defer func() {
			drainChannel(ch)
			callback()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-ch:
				if !ok {
					return
				}
				if resp.Error != nil {
					errCallback(resp.Error)
					return
				}
				if _, err := sha256Hash.Write(resp.Response.Data); err != nil {
					errCallback(fmt.Errorf("WriterChan: error writing to sha256 hash: %v", err))
					return
				}
				// The checksum is sent as the last packet
				if resp.Response.Checksum != nil {
					localChecksum := sha256Hash.Sum(nil)
					if !bytes.Equal(localChecksum, resp.Response.Checksum) {
						errCallback(fmt.Errorf("WriterChan: checksum mismatch"))
					}
					return
				}
				if _, err := w.Write(resp.Response.Data); err != nil {
					errCallback(fmt.Errorf("WriterChan: write error: %v", err))
					return
				}
			}
		}
	}()
}

func drainChannel(ch <-chan wshrpc.RespOrErrorUnion[iochantypes.Packet]) {
	for range ch {
	}
}
