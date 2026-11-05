package identity_test

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/zeebo/xxh3"
)

// Synthetic 1354-mod input, for before/after scan matching costs (16 §10).
func BenchmarkReconcile1354(b *testing.B) {
	for _, replacement := range []bool{false, true} {
		b.Run(fmt.Sprintf("replacement=%v", replacement), func(b *testing.B) {
			var known []*identity.Known
			var obs []*scan.Observation
			sets := map[string]scan.SemanticSet{}
			for n := 0; n < 1354; n++ {
				id := fmt.Sprintf("old%d", n)
				pkg := fmt.Sprintf("tester.old%d", n)
				files := scan.SemanticSet{}
				for f := 0; f < 4; f++ {
					files[fmt.Sprintf("Defs/%d.xml", f)] = xxh3.Hash128([]byte(fmt.Sprintf("old %d/%d", n, f))).Bytes()
				}
				sets[id] = files
				known = append(known, &identity.Known{ID: id, LineageID: "lineage" + id, SourceID: "local", Kind: source.Local, Rel: id, Status: identity.Live, Snap: &identity.Snap{Decl: scan.Decl{PackageID: pkg}}})
				rel := id
				if replacement {
					rel = fmt.Sprintf("new%d", n)
					pkg = fmt.Sprintf("tester.new%d", n)
					files = scan.SemanticSet{}
					for f := 0; f < 4; f++ {
						files[fmt.Sprintf("Defs/%d.xml", f)] = xxh3.Hash128([]byte(fmt.Sprintf("new %d/%d", n, f))).Bytes()
					}
				}
				obs = append(obs, &scan.Observation{Loc: scan.Location{SourceID: "local", Kind: source.Local, Rel: rel}, Decl: scan.Decl{PackageID: pkg}, FP: scan.Fingerprint{Semantic: files}})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copies := make([]*identity.Known, len(known))
				for j, k := range known {
					v := *k
					copies[j] = &v
				}
				next := 0
				identity.Reconcile(identity.Input{Obs: obs, Known: copies, NewID: func() string { next++; return fmt.Sprint(next) }, Semantic: func(k *identity.Known) scan.SemanticSet { return sets[k.ID] }})
			}
		})
	}
}
