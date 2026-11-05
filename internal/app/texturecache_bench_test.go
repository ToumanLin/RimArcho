package app

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/textures"
)

// 18 §2: opt-in, real-data benchmark; only the disposable data directory is written.
// RIMARCHO_TEXTURE_PERF_HOME=<tmp data dir> go test ./internal/app -run ^$ -bench BenchmarkTextureSummary -benchtime=1x -count=3
func BenchmarkTextureSummary(b *testing.B) {
	home := os.Getenv("RIMARCHO_TEXTURE_PERF_HOME")
	if home == "" {
		b.Skip("set RIMARCHO_TEXTURE_PERF_HOME to disposable real-data home")
	}
	a, err := Open(home)
	if err != nil {
		b.Fatal(err)
	}
	defer a.Close()
	targets, err := a.TextureTargets(nil, TexPlan)
	if err != nil {
		b.Fatal(err)
	}
	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				if _, err := a.textureSummaries(context.Background(), targets, textures.Options{}, nil, workers); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(targets)), "mods")
		})
	}
}
