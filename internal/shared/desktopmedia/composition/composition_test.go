package composition

import "testing"

func TestArchiveDialogProvider(t *testing.T) {
	if newArchiveDialogs(mediaParams{}) == nil {
		t.Fatal("missing archive selection adapter")
	}
}
