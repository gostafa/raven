package bbolt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	"raven/internal/shared/boltstore/ports"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
)

type sequentialBatch struct{}

// BenchmarkPersistence excludes fixture resets from time and database metrics.
func BenchmarkPersistence(b *testing.B) {
	for _, count := range []int{2, 64} {
		for _, operation := range []string{"list", "review", "acknowledge"} {
			b.Run(fmt.Sprintf("%s/items=%d", operation, count), func(b *testing.B) {
				benchmarkPersistence(b, operation, count)
			})
		}
	}
}

func persistenceFixture(tb testing.TB, count int) (*Store, []cardapi.Card) {
	tb.Helper()
	store, err := openStore(filepath.Join(tb.TempDir(), "raven.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := storeClose(store); err != nil {
			tb.Error(err)
		}
	})
	cards := make([]cardapi.Card, 4)
	for index := range cards {
		folder, err := storeCreateFolder(store,
			&folderapi.CreateInput{Title: fmt.Sprintf("Folder %d", index)},
		)
		if err != nil {
			tb.Fatal(err)
		}
		drafts := make([]contentapi.Draft, count)
		for position := range drafts {
			drafts[position] = contentapi.Draft{
				Kind: "text",
				Text: fmt.Sprintf("Item %d", position),
			}
		}
		cards[index], err = storeCreateCard(store,
			&cardapi.CreateInput{FolderID: folder.ID, Items: drafts},
		)
		if err != nil {
			tb.Fatal(err)
		}
	}
	return store, cards
}

func benchmarkPersistence(b *testing.B, operation string, count int) {
	b.Helper()
	store, cards := persistenceFixture(b, count)
	occurrences := make([]reviewapi.DueOccurrence, 0, len(cards)*count)
	for _, card := range cards {
		for _, item := range contentapi.Items(card.Items) {
			occurrences = append(occurrences, reviewapi.DueOccurrence{
				FolderID: card.FolderID,
				CardID:   card.ID,
				ItemID:   item.ID,
				Due:      contentapi.Schedule(item.Schedule).Due.UnixMilli(),
			})
		}
	}
	var pages, writes int64
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		before := store.db.Stats()
		switch operation {
		case "list":
			if _, err := storeListCards(store); err != nil {
				b.Fatal(err)
			}
		case "review":
			card := &cards[0]
			input := &cardapi.ReviewInput{
				FolderID: card.FolderID,
				CardID:   card.ID,
				ItemID:   contentapi.Items(card.Items)[0].ID,
				Rating:   3,
			}
			if _, err := storeReviewItemAt(store,
				input,
				contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).Due.Add(time.Second),
			); err != nil {
				b.Fatal(err)
			}
		case "acknowledge":
			if err := storeMarkNotified(store, occurrences); err != nil {
				b.Fatal(err)
			}
		}
		after := store.db.Stats()
		delta := after.Sub(&before)
		pages += delta.TxStats.GetPageCount()
		writes += delta.TxStats.GetWrite()
		b.StopTimer()
		if operation != "list" {
			resetPersistenceBenchmark(b, store, cards)
		}
		b.StartTimer()
	}
	b.StopTimer()
	b.ReportMetric(float64(pages)/float64(b.N), "pages/op")
	b.ReportMetric(float64(writes)/float64(b.N), "writes/op")
}

func resetPersistenceBenchmark(b *testing.B, store *Store, cards []cardapi.Card) {
	b.Helper()
	err := store.db.Update(func(tx *bolt.Tx) error {
		for index := range cards {
			card := &cards[index]
			bucket := tx.Bucket([]byte(foldersBucket)).Bucket([]byte(card.FolderID)).
				Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID))
			for itemIndex := range contentapi.Items(card.Items) {
				if err := writeContentItem(
					bucket,
					&contentapi.Items(card.Items)[itemIndex],
				); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func (sequentialBatch) Run(ctx context.Context, jobs []func(context.Context) error) error {
	for _, job := range jobs {
		if err := job(ctx); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkMediaImports(b *testing.B) {
	for _, size := range []int{4096, 1048576} {
		for _, count := range []int{1, 16} {
			for _, parallel := range []bool{false, true} {
				b.Run(
					fmt.Sprintf("bytes=%d/files=%d/parallel=%t", size, count, parallel),
					func(b *testing.B) {
						benchmarkImports(b, size, count, parallel)
					},
				)
			}
		}
	}
}

func benchmarkImports(b *testing.B, size, count int, parallel bool) {
	b.Helper()
	directory := b.TempDir()
	store, err := openStore(filepath.Join(directory, "raven.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := storeClose(store); err != nil {
			b.Error(err)
		}
	})
	var batch ports.BatchExecutor = sequentialBatch{}
	if parallel {
		batch = cffbatch.New()
	}
	store.batch = batch
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Benchmark"})
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, size)
	copy(data, "\x89PNG\r\n\x1a\n")
	drafts := make([]contentapi.Draft, count)
	for index := range drafts {
		path := filepath.Join(directory, fmt.Sprintf("source-%d.png", index))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			b.Fatal(err)
		}
		drafts[index] = contentapi.Draft{Kind: "image", SourcePath: path}
	}
	b.SetBytes(int64(size * count))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		card, err := storeCreateCard(store, &cardapi.CreateInput{FolderID: folder.ID, Items: drafts})
		if err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		if err := storeDeleteCard(store, folder.ID, card.ID); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}
