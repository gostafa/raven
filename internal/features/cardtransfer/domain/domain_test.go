package domain

import (
	"testing"

	transferapi "raven/api/cardtransfer"
)

func TestNormalizeTransfers(t *testing.T) {
	source := transferapi.Reference{FolderID: "folder", CardID: "card"}
	for _, operation := range []string{Copy, Move} {
		request := &transferapi.TransferInput{
			Operation:           operation,
			Sources:             []transferapi.Reference{source, source},
			DestinationFolderID: "destination",
		}
		if err := Normalize(
			request,
		); err != nil || len(request.Sources) != 1 ||
			request.Sources[0] != source {
			t.Fatal(request, err)
		}
	}
	for _, request := range []transferapi.TransferInput{
		{},
		{Operation: "invalid", Sources: []transferapi.Reference{source}, DestinationFolderID: "destination"},
		{Operation: Copy, Sources: []transferapi.Reference{source}},
		{Operation: Copy, DestinationFolderID: "destination"},
		{Operation: Copy, Sources: []transferapi.Reference{{FolderID: "folder"}}, DestinationFolderID: "destination"},
		{Operation: Copy, Sources: []transferapi.Reference{{CardID: "card"}}, DestinationFolderID: "destination"},
	} {
		if err := Normalize(&request); err == nil {
			t.Fatal("accepted invalid transfer", request)
		}
	}
}
