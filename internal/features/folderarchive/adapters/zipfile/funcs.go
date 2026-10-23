// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package zipfile

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	contentapi "raven/api/content"
	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
	"raven/internal/shared/filesystem"
)

// New constructs the ZIP adapter without acquiring files.
func New() *Archives { return &Archives{} }

// Write finalizes the chosen destination only after a complete, synced archive exists.
func (*Archives) Write(filename, manifest string, open ports.OpenMedia) error {
	return errors.Join(writeArchiveFile(filename, manifest, open))
}

func writeArchiveFile(filename, manifest string, open ports.OpenMedia) error {
	document, err := domain.Decode(manifest)
	if err != nil {
		return errors.Join(err)
	}

	file, err := os.CreateTemp(filepath.Dir(filename), ".raven-export-*")
	if err != nil {
		return fmt.Errorf("stage export: %w", err)
	}

	return errors.Join(
		saveArchive(file, &archiveWrite{document: document, open: open, filename: filename}),
	)
}

func saveArchive(file *os.File, request *archiveWrite) (failure error) {
	defer func() { failure = errors.Join(failure, removeTemporary(file.Name())) }()

	return errors.Join(finalizeExport(file, request.filename, func() error {
		return writeArchive(file, request.document, request.open)
	}))
}

func finalizeExport(file *os.File, filename string, write func() error) error {
	failure := write()

	failure = errors.Join(failure, file.Sync(), file.Close())
	if failure != nil {
		return fmt.Errorf("write archive: %w", failure)
	}

	err := os.Rename(file.Name(), filename)
	if err != nil {
		return fmt.Errorf("save archive: %w", err)
	}

	return nil
}

func removeTemporary(filename string) error {
	err := os.Remove(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return errors.Join(err)
}

func writeArchive(file *os.File, document *domain.Document, open ports.OpenMedia) error {
	writer := zip.NewWriter(file)

	failure := writeManifest(writer, document)
	if failure == nil {
		failure = writeMedia(writer, document, open)
	}

	return errors.Join(failure, writer.Close())
}

func writeManifest(writer *zip.Writer, document *domain.Document) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cards.json: %w", err)
	}

	if len(data) > domain.MaximumManifestSize {
		return domain.ErrInvalidArchive
	}

	return errors.Join(writeEntry(writer, domain.ManifestName, bytes.NewReader(data)))
}

func writeEntry(writer *zip.Writer, name string, source io.Reader) error {
	target, err := writer.Create(name)
	if err != nil {
		return fmt.Errorf("create ZIP entry: %w", err)
	}

	err = copyZIPFailure(io.Copy(target, source))
	if err != nil {
		return fmt.Errorf("write ZIP entry: %w", err)
	}

	return nil
}

func mediaReferences(document *domain.Document) (map[string]int64, error) {
	references := make(map[string]int64)

	for index := range document.Folders {
		err := folderReferences(&document.Folders[index], references)
		if err != nil {
			return nil, errors.Join(err)
		}
	}

	return references, nil
}

func folderReferences(folder *domain.Folder, references map[string]int64) error {
	for index := range folder.Cards {
		items, err := contentapi.Decode[[]contentapi.Item](folder.Cards[index].Items)
		if err != nil {
			return fmt.Errorf("decode archived items: %w", err)
		}

		err = itemReferences(items, references)
		if err != nil {
			return errors.Join(err)
		}
	}

	return nil
}

func itemReferences(items []contentapi.Item, references map[string]int64) error {
	for index := range items {
		err := referenceMedia(&items[index], references)
		if err != nil {
			return errors.Join(err)
		}
	}

	return nil
}

func referenceMedia(item *contentapi.Item, references map[string]int64) error {
	if item.RelativePath == "" {
		return nil
	}

	return errors.Join(addReference(item, references))
}

func addReference(item *contentapi.Item, references map[string]int64) error {
	if !validMediaReference(item) {
		return domain.ErrInvalidArchive
	}

	if _, exists := references[item.RelativePath]; exists {
		return domain.ErrInvalidArchive
	}

	references[item.RelativePath] = item.ByteSize

	return nil
}

func validMediaReference(item *contentapi.Item) bool {
	return domain.MediaName(item.RelativePath) && item.ByteSize > zero &&
		item.ByteSize <= domain.MaximumMediaSize
}

func writeMedia(writer *zip.Writer, document *domain.Document, open ports.OpenMedia) error {
	references, err := mediaReferences(document)
	if err != nil {
		return errors.Join(err)
	}

	err = validateTotals(references)
	if err != nil {
		return errors.Join(err)
	}

	return errors.Join(writeReferences(writer, references, open))
}

func validateTotals(references map[string]int64) error {
	var size int64

	for name := range references {
		size += references[name]
		if size > domain.MaximumArchiveSize {
			return domain.ErrInvalidArchive
		}
	}

	if len(references) >= domain.MaximumEntries {
		return domain.ErrInvalidArchive
	}

	return nil
}

func writeReferences(writer *zip.Writer, references map[string]int64, open ports.OpenMedia) error {
	for name := range references {
		err := writeMediaEntry(writer, &mediaEntry{name: name, size: references[name]}, open)
		if err != nil {
			return errors.Join(err)
		}
	}

	return nil
}

func writeMediaEntry(writer *zip.Writer, entry *mediaEntry, open ports.OpenMedia) error {
	source, err := open(entry.name)
	if err != nil {
		return fmt.Errorf("open exported media: %w", err)
	}

	limited := &io.LimitedReader{R: source, N: entry.size + one}
	failure := writeEntry(writer, entry.name, limited)

	if limited.N != one {
		failure = errors.Join(failure, domain.ErrInvalidArchive)
	}

	return errors.Join(failure, source.Close())
}

// Read keeps the selected archive open until its records and streamed media are consumed.
func (*Archives) Read(filename string, consume ports.Consume) error {
	return errors.Join(readArchiveFile(filename, consume))
}

func readArchiveFile(filename string, consume ports.Consume) error {
	file, err := filesystem.OpenSelected(filename)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}

	failure := readArchive(file, consume)

	return errors.Join(failure, file.Close())
}

func readArchive(file *os.File, consume ports.Consume) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect archive: %w", err)
	}

	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("read ZIP archive: %w", err)
	}

	return errors.Join(consumeArchive(reader, consume))
}

func consumeArchive(reader *zip.Reader, consume ports.Consume) error {
	index, err := indexArchive(reader)
	if err != nil {
		return errors.Join(err)
	}

	document, err := archiveReaderManifest(index)
	if err != nil {
		return errors.Join(err)
	}

	return errors.Join(archiveReaderConsume(index, document, consume))
}

func indexArchive(reader *zip.Reader) (*archiveReader, error) {
	if len(reader.File) > domain.MaximumEntries {
		return nil, domain.ErrInvalidArchive
	}

	index := &archiveReader{files: make(map[string]*zip.File)}

	for position := range reader.File {
		err := archiveReaderAdd(index, reader.File[position])
		if err != nil {
			return nil, errors.Join(err)
		}
	}

	return index, nil
}

func archiveReaderAdd(index *archiveReader, file *zip.File) error {
	if !validArchiveEntry(file) {
		return domain.ErrInvalidArchive
	}

	if _, exists := index.files[file.Name]; exists {
		return domain.ErrInvalidArchive
	}

	index.files[file.Name] = file

	return nil
}

func validArchiveEntry(file *zip.File) bool {
	if file.Name == mediaDirectory && file.Mode().IsDir() {
		return true
	}

	return file.Mode().IsRegular() && validEntryName(file.Name)
}

func validEntryName(name string) bool {
	return name == domain.ManifestName || domain.MediaName(name)
}

func archiveReaderManifest(index *archiveReader) (*domain.Document, error) {
	file := index.files[domain.ManifestName]
	if file == nil || file.UncompressedSize64 > domain.MaximumManifestSize {
		return nil, domain.ErrInvalidArchive
	}

	document, err := readManifestFile(file)
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

func readManifestFile(file *zip.File) (*domain.Document, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open cards.json: %w", err)
	}

	document, failure := decodeManifest(io.LimitReader(reader, domain.MaximumManifestSize+one))

	err = errors.Join(failure, reader.Close())
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

func decodeManifest(reader io.Reader) (*domain.Document, error) {
	data, err := readManifestBytes(reader)
	if err != nil {
		return nil, errors.Join(err)
	}

	document, err := parseManifest(data)
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

func readManifestBytes(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read cards.json: %w", err)
	}

	if len(data) > domain.MaximumManifestSize {
		return nil, domain.ErrInvalidArchive
	}

	return data, nil
}

func parseManifest(data []byte) (*domain.Document, error) {
	document, err := domain.Decode(string(data))
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

func archiveReaderConsume(
	index *archiveReader,
	document *domain.Document,
	consume ports.Consume,
) error {
	references, err := mediaReferences(document)
	if err != nil {
		return errors.Join(err)
	}

	err = archiveReaderValidateReferences(index, references)
	if err != nil {
		return errors.Join(err)
	}

	return errors.Join(consumeManifest(document, archiveReaderMedia(index), consume))
}

func consumeManifest(document *domain.Document, open ports.OpenMedia, consume ports.Consume) error {
	manifest, err := domain.Encode(document)
	if err != nil {
		return errors.Join(err)
	}

	return errors.Join(consume(manifest, open))
}

func archiveReaderValidateReferences(index *archiveReader, references map[string]int64) error {
	if len(index.files) != expectedEntries(index, len(references)) {
		return domain.ErrInvalidArchive
	}

	for name := range references {
		file := index.files[name]
		if file == nil || !validEntrySize(file, references[name]) {
			return domain.ErrInvalidArchive
		}
	}

	return errors.Join(validateTotals(references))
}

func expectedEntries(index *archiveReader, mediaCount int) int {
	expected := mediaCount + one

	if _, present := index.files[mediaDirectory]; present {
		expected++
	}

	return expected
}

func validEntrySize(file *zip.File, size int64) bool {
	if size < zero || size > domain.MaximumMediaSize {
		return false
	}

	return file.UncompressedSize64 == uint64(size)
}

func archiveReaderMedia(index *archiveReader) ports.OpenMedia {
	return func(name string) (io.ReadCloser, error) {
		file := index.files[name]
		if file == nil || !domain.MediaName(name) {
			return nil, domain.ErrInvalidArchive
		}

		reader, err := file.Open()

		return reader, errors.Join(err)
	}
}

func copyZIPFailure(_ int64, err error) error { return errors.Join(err) }
