// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

// NewReader supplies ServeContent with an independent cursor over shared immutable chunks.
func NewReader(ctx context.Context, source *Source) *Reader {
	owned := *source

	return &Reader{state: &readerState{
		chunk: nil, start: zero,
		load: func(start int64) ([]byte, error) { return loadChunk(ctx, &owned, start) },
		size: owned.Size, offset: zero,
	}}
}

// Version identifies the current opened file without exposing local paths in validators.
func Version(path string, info os.FileInfo) string {
	value := fmt.Sprintf(
		"%s:%s:%d:%d",
		path,
		fileIdentity(info),
		info.Size(),
		info.ModTime().UnixNano(),
	)

	return fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(value)))
}

// Read copies cached payloads so callers cannot modify shared entries.
func (reader *Reader) Read(target []byte) (int, error) {
	if len(target) == zero {
		return zero, nil
	}

	if reader.state.offset >= reader.state.size {
		return zero, io.EOF
	}

	count, err := reader.readChunk(target)

	return count, errors.Join(err)
}

func (reader *Reader) readChunk(target []byte) (int, error) {
	start := reader.state.offset / chunkSize * chunkSize

	data, err := reader.chunk(start)
	if err != nil {
		return zero, fmt.Errorf("read media chunk: %w", err)
	}

	count := copy(target, data[reader.state.offset-start:])

	reader.state.offset += int64(count)

	return count, nil
}

func (reader *Reader) chunk(start int64) ([]byte, error) {
	if reader.state.chunk != nil && reader.state.start == start {
		return reader.state.chunk, nil
	}

	data, err := reader.state.load(start)
	if err == nil {
		reader.state.chunk, reader.state.start = data, start
	}

	return data, errors.Join(err)
}

func loadChunk(ctx context.Context, source *Source, start int64) ([]byte, error) {
	load := func(ctx context.Context) ([]byte, error) { return retryReadAt(ctx, source, start) }

	if source.Cache == nil {
		value, err := load(ctx)

		return value, errors.Join(err)
	}

	key := source.Version + ":" + strconv.FormatInt(start, decimalBase)
	data, err := source.Cache.Get(ctx, key, load)

	return data, errors.Join(err)
}

func retryReadAt(ctx context.Context, source *Source, start int64) ([]byte, error) {
	if source.Retry == nil {
		value, err := readAt(ctx, source, start)

		return value, errors.Join(err)
	}

	var data []byte

	err := source.Retry.Do(ctx, func(ctx context.Context) error {
		value, err := readAt(ctx, source, start)
		if err == nil {
			data = value
		}

		return errors.Join(err)
	})

	return data, errors.Join(err)
}

func readAt(ctx context.Context, source *Source, start int64) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("load media chunk: %w", err)
	}

	length := min(chunkSize, source.Size-start)
	data := make([]byte, zero, length)[:length]
	count, err := source.File.ReadAt(data, start)

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.Join(err)
	}

	return data, errors.Join(validateRead(count, len(data), err))
}

func validateRead(count, length int, err error) error {
	if count != length {
		return fmt.Errorf(
			"read incomplete media chunk: %w",
			errors.Join(io.ErrUnexpectedEOF, err),
		)
	}

	return nil
}

// Seek preserves standard streaming and byte-range behavior without reading any bytes.
func (reader *Reader) Seek(offset int64, whence int) (int64, error) {
	position, valid := reader.position(offset, whence)
	if !valid || position < zero {
		return reader.state.offset, errInvalidSeek
	}

	reader.state.offset = position

	return position, nil
}

func (reader *Reader) position(offset int64, whence int) (int64, bool) {
	switch whence {
	case io.SeekStart:
		return offset, true
	case io.SeekCurrent:
		return reader.state.offset + offset, true
	case io.SeekEnd:
		return reader.state.size + offset, true
	default:
		return zero, false
	}
}
