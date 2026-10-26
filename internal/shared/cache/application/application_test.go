package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	cacheotter "raven/internal/shared/cache/adapters/otter"
	resilience "raven/internal/shared/resilience/adapters/retry"
)

type countedReader struct {
	source *bytes.Reader
	reads  atomic.Int64
}

type syntheticInfo struct{ os.FileInfo }

type recoveringReader struct {
	calls   int
	failure error
}

func (reader *recoveringReader) ReadAt(data []byte, _ int64) (int, error) {
	reader.calls++
	if reader.calls == 1 {
		copy(data, "bad")
		return 1, reader.failure
	}
	return copy(data, "abc"), nil
}

func TestChunkRetriesDiscardPartialBuffersAndCacheOnlySuccess(t *testing.T) {
	source, _ := chunkFixture(t, []byte("abc"))
	backing := &recoveringReader{failure: syscall.EAGAIN}
	source.File = backing
	source.Retry = resilience.NewReads(t.Context(), nil)
	for range 2 {
		data, err := io.ReadAll(NewReader(t.Context(), source))
		if err != nil || string(data) != "abc" {
			t.Fatal(string(data), err)
		}
	}
	if backing.calls != 2 {
		t.Fatal("successful chunk not reused", backing.calls)
	}
	for _, failure := range []error{os.ErrNotExist, io.ErrUnexpectedEOF} {
		backing := &recoveringReader{failure: failure}
		_, err := io.ReadAll(
			NewReader(t.Context(), &Source{File: backing, Size: 3, Retry: source.Retry}),
		)
		if !errors.Is(err, failure) || backing.calls != 1 {
			t.Fatal(backing.calls, err)
		}
	}
}

func (syntheticInfo) Sys() any { return nil }

func (source *countedReader) ReadAt(target []byte, offset int64) (int, error) {
	source.reads.Add(1)
	return source.source.ReadAt(target, offset)
}

type failingReader struct {
	failure error
	full    bool
}

func (source failingReader) ReadAt(target []byte, _ int64) (int, error) {
	if source.full {
		return len(target), source.failure
	}
	return 0, source.failure
}

func chunkFixture(t testing.TB, data []byte) (*Source, *countedReader) {
	t.Helper()
	cache := cacheotter.New(
		&cacheotter.Options[string, []byte]{
			MaximumWeight: 1024 * 1024,
			Weigher:       func(_ string, value []byte) uint32 { return uint32(len(value)) },
		},
	)
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	backing := &countedReader{source: bytes.NewReader(data)}
	return &Source{Cache: cache, File: backing, Version: "version", Size: int64(len(data))}, backing
}

func TestChunkReaderConcurrentReuseAndIndependentCursors(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefg"), 100000)
	source, backing := chunkFixture(t, data)
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			reader := NewReader(t.Context(), source)
			if _, err := reader.Seek(chunkSize-10, io.SeekStart); err != nil {
				t.Error(err)
				return
			}
			got := make([]byte, 40)
			if _, err := io.ReadFull(
				reader,
				got,
			); err != nil ||
				!bytes.Equal(got, data[chunkSize-10:chunkSize+30]) {
				t.Errorf("read=%q err=%v", got, err)
			}
		})
	}
	workers.Wait()
	if backing.reads.Load() != 2 {
		t.Fatalf("backing reads=%d, want 2", backing.reads.Load())
	}
	reader := NewReader(t.Context(), source)
	all, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(all, data) {
		t.Fatal("stream differs", err)
	}
	if backing.reads.Load() != 3 {
		t.Fatal("did not reuse earlier chunks")
	}
	all[0] = '!'
	reader = NewReader(t.Context(), source)
	first := make([]byte, 1)
	_, _ = reader.Read(first)
	if first[0] != data[0] {
		t.Fatal("caller mutated shared payload")
	}
	source.Version = "new-version"
	_, _ = io.ReadAll(NewReader(t.Context(), source))
	if backing.reads.Load() != 6 {
		t.Fatal("file version reused old chunks")
	}
}

func TestChunkReaderSeekEOFAndFailures(t *testing.T) {
	source := &Source{File: bytes.NewReader([]byte("abc")), Version: "plain", Size: 3}
	reader := NewReader(t.Context(), source)
	for _, step := range []struct {
		offset  int64
		whence  int
		want    int64
		invalid bool
	}{
		{1, io.SeekStart, 1, false},
		{1, io.SeekCurrent, 2, false},
		{-1, io.SeekEnd, 2, false},
		{-4, io.SeekEnd, 2, true},
		{0, 100, 2, true},
		{math.MaxInt64, io.SeekCurrent, 2, true},
		{5, io.SeekStart, 5, false},
	} {
		got, err := reader.Seek(step.offset, step.whence)
		if got != step.want || (err != nil) != step.invalid {
			t.Fatalf("seek %+v: %d %v", step, got, err)
		}
	}
	if n, err := reader.Read(nil); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := reader.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatal(n, err)
	}
	_, _ = reader.Seek(0, io.SeekStart)
	if got, err := io.ReadAll(reader); err != nil || string(got) != "abc" {
		t.Fatal(string(got), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewReader(ctx, source).Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	failure := errors.New("disk failed")
	for _, full := range []bool{false, true} {
		source.File = failingReader{failure: failure, full: full}
		if _, err := NewReader(
			t.Context(),
			source,
		).Read(make([]byte, 1)); !errors.Is(
			err,
			failure,
		) {
			t.Fatal(err)
		}
	}
	source.File = failingReader{failure: io.EOF, full: true}
	if _, err := NewReader(t.Context(), source).Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	source.File = bytes.NewReader([]byte("a"))
	if _, err := NewReader(
		t.Context(),
		source,
	).Read(make([]byte, 1)); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
}

func TestFileValidatorsChangeWithFileVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := Version(path, info)
	if fileIdentity(syntheticInfo{info}) != info.Name() {
		t.Fatal("synthetic file identity")
	}
	if before != Version(path, info) || before == Version(path+"other", info) {
		t.Fatal("unstable file identity")
	}
	if err := os.Chtimes(path, time.Now(), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before == Version(path, info) {
		t.Fatal("validator did not change")
	}
}

func TestRequestRetainsCurrentChunkWhenSharedStorageIsUnavailable(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 700000)
	backing := &countedReader{source: bytes.NewReader(data)}
	source := &Source{File: backing, Size: int64(len(data)), Version: "uncached"}
	var result bytes.Buffer
	if _, err := io.CopyBuffer(
		&result,
		NewReader(t.Context(), source),
		make([]byte, 8192),
	); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes(), data) || backing.reads.Load() != 3 {
		t.Fatalf("reads=%d", backing.reads.Load())
	}
}
