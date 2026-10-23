package zipfile

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
)

func zipDocument() *domain.Document {
	return &domain.Document{
		Version:    domain.Version,
		ExportedAt: time.Now().UTC(),
		Folders: []domain.Folder{
			{
				Metadata: folderapi.Folder{ID: "folder", Title: "Archive"},
				Cards: []cardapi.Card{
					{
						Items: []contentapi.Item{
							{ID: "item", RelativePath: "media/item.png", ByteSize: 4},
						},
					},
				},
			},
		},
	}
}

func TestArchiveZIPRoundTrip(t *testing.T) {
	adapter := New()
	filename := filepath.Join(t.TempDir(), "test.raven.zip")
	document := zipDocument()
	open := func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("data")), nil }
	if err := adapter.Write(filename, encodedZIP(t, document), open); err != nil {
		t.Fatal(err)
	}
	err := adapter.Read(filename, func(manifest string, media ports.OpenMedia) error {
		imported, err := domain.Decode(manifest)
		if err != nil {
			return err
		}
		if imported.Version != document.Version ||
			!imported.ExportedAt.Equal(document.ExportedAt) ||
			len(imported.Folders) != 1 {
			t.Fatal("lost manifest fields")
		}
		reader, err := media("media/item.png")
		if err != nil {
			return err
		}
		data, failure := io.ReadAll(reader)
		if string(data) != "data" {
			t.Fatal("lost media")
		}
		return errors.Join(failure, reader.Close())
	})
	if err != nil {
		t.Fatal(err)
	}
}

type zipEntry struct {
	name string
	data []byte
	mode os.FileMode
}

func archiveFile(t *testing.T, entries []zipEntry) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "fixture.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		target, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = target.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err = errors.Join(writer.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestArchiveRejectsInvalidZIPEntries(t *testing.T) {
	manifest, err := json.Marshal(zipDocument())
	if err != nil {
		t.Fatal(err)
	}
	for _, entries := range [][]zipEntry{
		{},
		{{name: "cards.json", data: []byte("{")}},
		{{name: "cards.json", data: append(append([]byte{}, manifest...), []byte("{}")...)}},
		{{name: "cards.json", data: manifest}},
		{{name: "cards.json", data: manifest}, {name: "cards.json", data: manifest}},
		{{name: "cards.json", data: manifest}, {name: "../outside", data: []byte("bad")}},
		{{name: "cards.json", data: manifest}, {name: "media/item.png", mode: os.ModeSymlink | 0o600}},
		{{name: "cards.json", data: manifest}, {name: "media/item.png", data: []byte("wrong size")}},
		{{name: "cards.json", data: manifest}, {name: "media/item.png", data: []byte("data")}, {name: "media/extra.png", data: []byte("data")}},
	} {
		called := false
		err := New().Read(archiveFile(t, entries), func(string, ports.OpenMedia) error { called = true; return nil })
		if err == nil || called {
			t.Fatalf("accepted malformed archive: %+v", entries)
		}
	}
	if _, err := parseManifest(
		[]byte(`{"version":9,"exported_at":"2026-01-01T00:00:00Z","folders":[{}]}`),
	); err == nil {
		t.Fatal("unsupported version")
	}
	if _, err := parseManifest([]byte(`{"unknown":true}`)); err == nil {
		t.Fatal("unknown manifest fields")
	}
}

func TestArchiveMediaChecksumAndNames(t *testing.T) {
	manifest, err := json.Marshal(zipDocument())
	if err != nil {
		t.Fatal(err)
	}
	filename := archiveFile(
		t,
		[]zipEntry{
			{name: "cards.json", data: manifest},
			{name: "media/", mode: os.ModeDir | 0o700},
			{name: "media/item.png", data: []byte("data")},
		},
	)
	err = New().Read(filename, func(_ string, open ports.OpenMedia) error {
		if _, err := open("../outside"); err == nil {
			t.Fatal("opened unsafe name")
		}
		if _, err := open("media/missing.png"); err == nil {
			t.Fatal("opened absent file")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[2].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 1
	if err = os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	err = New().Read(filename, func(_ string, open ports.OpenMedia) error {
		file, err := open("media/item.png")
		if err != nil {
			return err
		}
		_, failure := io.Copy(io.Discard, file)
		return errors.Join(failure, file.Close())
	})
	if !errors.Is(err, zip.ErrChecksum) {
		t.Fatal("accepted corrupt media", err)
	}
}

func TestArchiveFailedExportPreservesDestination(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "existing.zip")
	if err := os.WriteFile(filename, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, open := range []ports.OpenMedia{
		func(string) (io.ReadCloser, error) { return nil, io.ErrUnexpectedEOF },
		func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("shorter or longer")), nil },
		func(string) (io.ReadCloser, error) { return &brokenReader{}, nil },
	} {
		if err := New().Write(filename, encodedZIP(t, zipDocument()), open); err == nil {
			t.Fatal("export succeeded despite media failure")
		}
		data, err := os.ReadFile(filename)
		if err != nil || string(data) != "existing" {
			t.Fatal("destination was changed", err)
		}
		entries, err := os.ReadDir(filepath.Dir(filename))
		if err != nil || len(entries) != 1 {
			t.Fatal("temporary file survived", err)
		}
	}
}

type brokenReader struct{}

func (*brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (*brokenReader) Close() error             { return io.ErrClosedPipe }

func TestArchiveBoundsAndIOFailures(t *testing.T) {
	if err := New().Write(filepath.Join(t.TempDir(), "missing", "test.zip"), encodedZIP(t, zipDocument()), nil); err == nil {
		t.Fatal("missing destination")
	}
	if err := New().Read(filepath.Join(t.TempDir(), "missing.zip"), nil); err == nil {
		t.Fatal("missing archive")
	}
	file, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = readArchive(file, nil); err == nil {
		t.Fatal("stat closed file")
	}
	if _, err = decodeManifest(&brokenReader{}); err == nil {
		t.Fatal("manifest read failure")
	}
	if _, err = decodeManifest(
		strings.NewReader(strings.Repeat(" ", domain.MaximumManifestSize+1)),
	); err == nil {
		t.Fatal("oversized JSON")
	}
	if err = validateTotals(map[string]int64{"a": domain.MaximumArchiveSize + 1}); err == nil {
		t.Fatal("expanded size limit")
	}
	tooMany := make(map[string]int64)
	for index := range domain.MaximumEntries {
		tooMany[strconv.Itoa(index)] = 1
	}
	if err = validateTotals(tooMany); err == nil {
		t.Fatal("entry limit")
	}
	for _, item := range []contentapi.Item{{RelativePath: "../bad", ByteSize: 1}, {RelativePath: "media/a", ByteSize: 0}, {RelativePath: "media/a", ByteSize: domain.MaximumMediaSize + 1}} {
		if err = addReference(&item, map[string]int64{}); err == nil {
			t.Fatal("invalid reference")
		}
	}
	item := contentapi.Item{RelativePath: "media/a", ByteSize: 1}
	if err = addReference(&item, map[string]int64{"media/a": 1}); err == nil {
		t.Fatal("duplicate reference")
	}
	document := zipDocument()
	document.Folders[0].Cards[0].Items = make(chan int)
	if _, err = mediaReferences(document); err == nil {
		t.Fatal("invalid items")
	}
	if reflect.DeepEqual(document, zipDocument()) {
		t.Fatal("fixture should differ")
	}
}

func encodedZIP(t *testing.T, document *domain.Document) string {
	t.Helper()
	manifest, err := domain.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestArchiveAdditionalWriteFailures(t *testing.T) {
	if err := referenceMedia(&contentapi.Item{Text: "hello"}, map[string]int64{}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := New().Write(filepath.Join(directory, "bad.zip"), "invalid", nil); err == nil {
		t.Fatal("invalid export manifest")
	}
	file, err := os.CreateTemp(directory, "rename")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "destination")
	if err = os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = finalizeExport(file, target, func() error { return nil }); err == nil {
		t.Fatal("rename onto directory")
	}
	if err = removeTemporary(target); err == nil {
		t.Fatal("nonempty directory cleanup")
	}
	if err = removeTemporary(filepath.Join(directory, "absent")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	if err = writeEntry(writer, strings.Repeat("x", 1<<16), strings.NewReader("x")); err == nil {
		t.Fatal("invalid ZIP filename")
	}
	document := zipDocument()
	document.Folders[0].Cards[0].Items = make(chan int)
	if err = writeManifest(writer, document); err == nil {
		t.Fatal("unencodable manifest")
	}
	if err = consumeManifest(document, nil, nil); err == nil {
		t.Fatal("unencodable consumed manifest")
	}
	document.Folders[0].Cards[0].Items = []contentapi.Item{
		{Text: strings.Repeat("x", domain.MaximumManifestSize)},
	}
	if err = writeManifest(writer, document); err == nil {
		t.Fatal("oversized manifest")
	}
	document.Folders[0].Cards[0].Items = []contentapi.Item{{RelativePath: "../unsafe", ByteSize: 1}}
	if err = writeMedia(writer, document, nil); err == nil {
		t.Fatal("unsafe media reference")
	}
	if err = archiveReaderConsume(&archiveReader{}, document, nil); err == nil {
		t.Fatal("invalid consumed media reference")
	}
	items := make([]contentapi.Item, 5)
	for index := range items {
		items[index] = contentapi.Item{
			RelativePath: "media/" + strconv.Itoa(index) + ".mp4",
			ByteSize:     domain.MaximumMediaSize,
		}
	}
	document.Folders[0].Cards[0].Items = items
	if err = writeMedia(writer, document, nil); err == nil {
		t.Fatal("expanded size limit on export")
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveAdditionalReadFailures(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(filename, []byte("not ZIP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New().Read(filename, nil); err == nil {
		t.Fatal("invalid ZIP")
	}
	if _, err := indexArchive(
		&zip.Reader{File: make([]*zip.File, domain.MaximumEntries+1)},
	); err == nil {
		t.Fatal("entry count limit")
	}
	reader, err := zip.OpenReader(
		archiveFile(t, []zipEntry{{name: domain.ManifestName, data: []byte("{}")}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	file := reader.File[0]
	file.Method = 99
	index := &archiveReader{files: map[string]*zip.File{domain.ManifestName: file}}
	if _, err := archiveReaderManifest(index); err == nil {
		t.Fatal("unsupported manifest compression")
	}
	file.UncompressedSize64 = domain.MaximumManifestSize + 1
	if _, err := archiveReaderManifest(index); err == nil {
		t.Fatal("oversized declared manifest")
	}
	file.Name = "media/item.png"
	index.files[file.Name] = file
	if _, err := archiveReaderMedia(index)(file.Name); err == nil {
		t.Fatal("unsupported media compression")
	}
	if validEntrySize(file, -1) || validEntrySize(file, domain.MaximumMediaSize+1) {
		t.Fatal("invalid entry size")
	}
	if err := archiveReaderAdd(
		index,
		&zip.File{FileHeader: zip.FileHeader{Name: "unexpected/"}},
	); err == nil {
		t.Fatal("unexpected directory")
	}
}
