package application

import (
	"errors"
	"testing"
)

type transferLibraryStub struct {
	failure error
	calls   int
}

func (library *transferLibraryStub) ApplyCardTransfer(string) (string, error) {
	library.calls++
	return `{"entries":[]}`, library.failure
}

func TestTransferWorkflowValidatesAndReturnsMappings(t *testing.T) {
	library := &transferLibraryStub{}
	service := New(library)
	input := `{"operation":"copy","sources":[{"folder_id":"folder","card_id":"card"}],"destination_folder_id":"destination"}`
	response, err := service.TransferCards(input)
	if err != nil || response != `{"entries":[]}` || library.calls != 1 {
		t.Fatal(response, err)
	}
	for _, invalid := range []string{"{", "{}", "null"} {
		if _, err = service.TransferCards(invalid); err == nil {
			t.Fatal("invalid transfer accepted")
		}
	}
	if library.calls != 1 {
		t.Fatal("invalid input reached persistence")
	}
	library.failure = errors.New("disk failed")
	if _, err = service.TransferCards(input); !errors.Is(err, library.failure) {
		t.Fatal("lost persistence failure", err)
	}
}
