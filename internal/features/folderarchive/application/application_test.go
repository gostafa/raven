package application

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
)

type workflowFixture struct {
	document *domain.Document
	filename string
	failure  error
	ids      []string
	mode     string
	calls    int
}

func (fixture *workflowFixture) SaveArchive() (string, error) {
	return fixture.filename, fixture.failure
}

func (fixture *workflowFixture) OpenArchive() (string, error) {
	return fixture.filename, fixture.failure
}

func (fixture *workflowFixture) SnapshotFolders(ids []string, consume ports.Consume) error {
	fixture.ids = ids
	fixture.calls++
	manifest, err := domain.Encode(fixture.document)
	if err != nil {
		return err
	}
	return consume(manifest, nil)
}

func (fixture *workflowFixture) ImportFolderCopies(
	_ string,
	mode string,
	_ ports.OpenMedia,
) ([]string, error) {
	fixture.mode = mode
	fixture.calls++
	return []string{"new-folder"}, fixture.failure
}

func (fixture *workflowFixture) Write(string, string, ports.OpenMedia) error {
	return fixture.failure
}

func (fixture *workflowFixture) Read(_ string, consume ports.Consume) error {
	manifest, err := domain.Encode(fixture.document)
	if err != nil {
		return err
	}
	return consume(manifest, nil)
}

func workflowDocument() *domain.Document {
	return &domain.Document{
		Version:    domain.Version,
		ExportedAt: time.Now(),
		Folders: []domain.Folder{
			{
				Cards: []cardapi.Card{
					{
						Items: []contentapi.Item{
							{Kind: "text", Text: "Hello"},
							{RelativePath: "media/file.png"},
						},
					},
				},
			},
		},
	}
}

func TestArchiveWorkflows(t *testing.T) {
	fixture := &workflowFixture{
		document: workflowDocument(),
		filename: filepath.Join(t.TempDir(), "test.raven.zip"),
	}
	service := New(fixture, fixture, fixture)
	result, err := service.ExportFolders(`{"folder_ids":["first","second"]}`)
	if err != nil || result.Canceled || result.FolderCount != 1 || result.CardCount != 1 ||
		result.MediaCount != 1 ||
		result.Filename != "test.raven.zip" {
		t.Fatalf("export result: %+v %v", result, err)
	}
	if !reflect.DeepEqual(fixture.ids, []string{"first", "second"}) {
		t.Fatal(fixture.ids)
	}
	for _, mode := range []string{domain.Fresh, domain.Resume} {
		result, err = service.ImportFolders(mode)
		if err != nil || fixture.mode != mode ||
			!reflect.DeepEqual(result.FolderIDs, []string{"new-folder"}) {
			t.Fatalf("import result: %+v %v", result, err)
		}
	}
}

func TestArchiveCancellationAndInvalidRequests(t *testing.T) {
	fixture := &workflowFixture{}
	service := New(fixture, fixture, fixture)
	for _, input := range []string{"{", `{}`, `{"folder_ids":[]}`} {
		if _, err := service.ExportFolders(input); err == nil {
			t.Fatal("accepted invalid export", input)
		}
	}
	if _, err := service.ImportFolders("unknown"); !errors.Is(err, domain.ErrInvalidMode) {
		t.Fatal(err)
	}
	for _, operation := range []func() (domain.Result, error){
		func() (domain.Result, error) { return service.ExportFolders(`{"folder_ids":["one"]}`) },
		func() (domain.Result, error) { return service.ImportFolders(domain.Fresh) },
	} {
		result, err := operation()
		if err != nil || !result.Canceled || fixture.calls != 0 {
			t.Fatalf("cancellation: %+v %v", result, err)
		}
	}
	fixture.failure = io.ErrUnexpectedEOF
	if _, err := service.ExportFolders(`{"folder_ids":["one"]}`); !errors.Is(err, fixture.failure) {
		t.Fatal(err)
	}
	if _, err := service.ImportFolders(domain.Resume); !errors.Is(err, fixture.failure) {
		t.Fatal(err)
	}
}

func TestArchiveWorkflowFailures(t *testing.T) {
	if _, err := archiveSummary("invalid"); err == nil {
		t.Fatal("invalid manifest summary")
	}
	fixture := &workflowFixture{
		filename: "test.zip",
		document: workflowDocument(),
		failure:  io.ErrUnexpectedEOF,
	}
	for _, items := range []any{fixture.document.Folders[0].Cards[0].Items, "invalid"} {
		fixture.document.Folders[0].Cards[0].Items = items
		request := archiveOperation{
			manifest: encodedWorkflow(t, fixture.document),
			filename: "test.zip",
			mode:     domain.Resume,
			result:   &domain.Result{},
		}
		if err := workflowExportSnapshot(&workflow{archives: fixture}, &request); err == nil {
			t.Fatal("failed export")
		}
		if err := workflowImportSnapshot(&workflow{library: fixture}, &request); err == nil {
			t.Fatal("failed import")
		}
	}
}

func encodedWorkflow(t *testing.T, document *domain.Document) string {
	t.Helper()
	manifest, err := domain.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
