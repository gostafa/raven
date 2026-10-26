package application

import (
	"bytes"
	"io"
	"testing"
)

func BenchmarkMediaChunks(b *testing.B) {
	for _, warm := range []bool{false, true} {
		b.Run(map[bool]string{false: "cold", true: "warm"}[warm], func(b *testing.B) {
			data := bytes.Repeat([]byte("x"), 1024*1024)
			source, backing := chunkFixture(b, data)
			if _, err := io.Copy(io.Discard, NewReader(b.Context(), source)); err != nil {
				b.Fatal(err)
			}
			backing.reads.Store(0)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if !warm {
					source.Cache.InvalidateAll()
				}
				if _, err := io.Copy(io.Discard, NewReader(b.Context(), source)); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(backing.reads.Load())/float64(b.N), "reads/op")
		})
	}
}
