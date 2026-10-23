package domain

import (
	"strings"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
)

func TestArchiveContracts(t *testing.T) {
	if ValidateMode(Fresh) != nil || ValidateMode(Resume) != nil || ValidateMode("") == nil {
		t.Fatal("import mode validation")
	}
	document := &Document{Version: Version, ExportedAt: time.Now(), Folders: []Folder{{}}}
	if ValidateEnvelope(document) != nil {
		t.Fatal("valid envelope")
	}
	for _, invalid := range []*Document{{}, {Version: 2}, {Version: Version, ExportedAt: time.Now()}} {
		if ValidateEnvelope(invalid) == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	for _, name := range []string{"media/file.png", "media/abcdef.wav"} {
		if !MediaName(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"../outside", "media/../file", "media/sub/file", "media/", "media/.", "media/c:\\file", "media/\x00file"} {
		if MediaName(name) {
			t.Fatal("unsafe name", name)
		}
	}
}

func TestManifestJSONAndSummary(t *testing.T) {
	document := &Document{
		Version:    Version,
		ExportedAt: time.Now(),
		Folders: []Folder{
			{
				Cards: []cardapi.Card{
					{
						Items: []contentapi.Item{
							{Kind: "text", Text: "Hello"},
							{RelativePath: "media/image.png"},
						},
					},
				},
			},
		},
	}
	manifest, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(manifest)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Summary(decoded)
	if err != nil || summary.FolderCount != 1 || summary.CardCount != 1 || summary.MediaCount != 1 {
		t.Fatal(summary, err)
	}
	for _, invalid := range []string{"{", "{}", `{"unexpected":true}`, strings.Repeat(" ", MaximumManifestSize+1)} {
		if _, err = Decode(invalid); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	document.Folders[0].Cards[0].Items = "invalid"
	if _, err = Summary(document); err == nil {
		t.Fatal("invalid summary accepted")
	}
	document.Folders[0].Cards[0].Items = make(chan int)
	if _, err = Encode(document); err == nil {
		t.Fatal("invalid JSON encoded")
	}
}
