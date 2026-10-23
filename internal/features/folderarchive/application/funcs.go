// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	folderapi "raven/api/folders"
	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
)

// New constructs archive workflows without acquiring resources.
func New(library ports.Library, archives ports.Archives, dialogs ports.Dialogs) *Service {
	worker := workflow{library: library, archives: archives, dialogs: dialogs}

	return &Service{operations: map[string]func(string) (domain.Result, error){
		"export": func(input string) (domain.Result, error) { return workflowExportFolders(&worker, input) },
		"import": func(mode string) (domain.Result, error) { return workflowImportFolders(&worker, mode) },
	}}
}

// ExportFolders exports the selected folder IDs after native destination selection.
func workflowExportFolders(service *workflow, input string) (domain.Result, error) {
	ids, err := exportIDs(input)
	if err != nil {
		return domain.Result{}, errors.Join(err)
	}

	result, err := workflowExportSelected(service, ids)

	return result, errors.Join(err)
}

func exportIDs(input string) ([]string, error) {
	var request folderapi.ExportInput

	err := json.Unmarshal([]byte(input), &request)
	if err != nil {
		return nil, fmt.Errorf("read folder selection: %w", err)
	}

	if len(request.FolderIDs) == zero || len(request.FolderIDs) > domain.MaximumEntries {
		return nil, domain.ErrInvalidArchive
	}

	return request.FolderIDs, nil
}

func workflowExportSelected(service *workflow, ids []string) (domain.Result, error) {
	filename, err := service.dialogs.SaveArchive()
	if err != nil || filename == "" {
		return canceledResult(filename), errors.Join(err)
	}

	var result domain.Result

	request := newArchiveOperation(filename, "", &result)
	consumer := archiveConsumer(service, request, workflowExportSnapshot)

	err = service.library.SnapshotFolders(ids, consumer)

	return result, errors.Join(err)
}

func workflowExportSnapshot(service *workflow, request *archiveOperation) error {
	summary, err := archiveSummary(request.manifest)
	if err != nil {
		return errors.Join(err)
	}

	err = service.archives.Write(request.filename, request.manifest, request.open)
	if err != nil {
		return fmt.Errorf("export folders: %w", err)
	}

	summary.Filename = filepath.Base(request.filename)
	*request.result = summary

	return nil
}

// ImportFolders imports every archived folder as a separate copy with the chosen review mode.
func workflowImportFolders(service *workflow, mode string) (domain.Result, error) {
	err := domain.ValidateMode(mode)
	if err != nil {
		return domain.Result{}, errors.Join(err)
	}

	result, err := workflowImportSelected(service, mode)

	return result, errors.Join(err)
}

func workflowImportSelected(service *workflow, mode string) (domain.Result, error) {
	filename, err := service.dialogs.OpenArchive()
	if err != nil || filename == "" {
		return canceledResult(filename), errors.Join(err)
	}

	var result domain.Result

	request := newArchiveOperation(filename, mode, &result)
	consumer := archiveConsumer(service, request, workflowImportSnapshot)

	err = service.archives.Read(filename, consumer)
	result.Filename = filepath.Base(filename)

	return result, errors.Join(err)
}

func newArchiveOperation(filename, mode string, result *domain.Result) *archiveOperation {
	return &archiveOperation{
		filename: filename,
		mode:     mode,
		result:   result,
		manifest: "",
		open:     nil,
	}
}

func archiveConsumer(
	service *workflow,
	request *archiveOperation,
	consume func(*workflow, *archiveOperation) error,
) ports.Consume {
	return func(manifest string, open ports.OpenMedia) error {
		request.manifest = manifest
		request.open = open

		return errors.Join(consume(service, request))
	}
}

func workflowImportSnapshot(service *workflow, request *archiveOperation) error {
	summary, err := archiveSummary(request.manifest)
	if err != nil {
		return errors.Join(err)
	}

	summary.FolderIDs, err = service.library.ImportFolderCopies(
		request.manifest,
		request.mode,
		request.open,
	)
	if err != nil {
		return fmt.Errorf("import folders: %w", err)
	}

	*request.result = summary

	return nil
}

// ExportFolders exports the selected folder IDs using the native save dialog.
func (service *Service) ExportFolders(input string) (domain.Result, error) {
	result, err := service.operations["export"](input)

	return result, errors.Join(err)
}

// ImportFolders imports copies using the chosen review mode.
func (service *Service) ImportFolders(mode string) (domain.Result, error) {
	result, err := service.operations["import"](mode)

	return result, errors.Join(err)
}

func archiveSummary(manifest string) (domain.Result, error) {
	document, err := domain.Decode(manifest)
	if err != nil {
		return domain.Result{}, errors.Join(err)
	}

	result, err := domain.Summary(document)

	return result, errors.Join(err)
}

func canceledResult(filename string) domain.Result {
	var result domain.Result

	result.Canceled = filename == ""

	return result
}
