// Spike: scaffolding for labelling the real cause of log error clusters, so G1 attribution
// accuracy can be measured, not only coverage (docs/research/00 §6.4, §6.5, R1, R2).
//
//	corpuslabel sample   -results results-v3.jsonl -out sample.jsonl [-tpl 2 -per 2 -mincov 0.8 -seed s]
//	corpuslabel cooc     -results results-v3.jsonl -corpus C -sample sample.jsonl -out cooc.json
//	corpuslabel packets  -home H -corpus <RimArcho-corpus>\hugslogs -sample sample.jsonl -out <dir> [-cooc cooc.json] [-steamdb steamDB.json]
//	corpuslabel validate -sample sample.jsonl -packets <dir> -labels <dir> [-corpus C -execlogs <dir>]
//	corpuslabel score    -sample sample.jsonl -packets <dir> -labels <dir> -review <dir>
//	corpuslabel exec     -home H -corpus C (-gist <id> | -packet <item>.txt) [-log transcript] -- <archo command…>
//
// Benchmark v2 (G6, bench.go, SCHEMA.md):
//
//	corpuslabel bench-sample -results results-v4.jsonl -out <v2>\sample.jsonl [-n1 20 -n2 15 -mincov 0.8 -seed s -exclude pilot\sample.jsonl]
//	corpuslabel bench-score  -sample … -packets … -labels … -review … -rimarcho <v2>\rimarcho-v4.jsonl [-execlogs <dir> -costs costs.jsonl -corpus C]
//	corpuslabel tasks        -sample … -packets … -labels … -review … -out <v2> [-execlogs <dir> -corpus C]
//
// sample stratifies problem clusters by class and template (several per frequent template);
// packets loads each sampled log into a research environment (as spikes/corpusrun does) and
// writes one bounded evidence packet per cluster: <item>.json (all fields, including
// RimArcho's attribution) and <item>.txt (what the labeller reads; blind to RimArcho's
// verdict). Labels follow schema.go; validate checks them against the sample and packets;
// score compares labeller vs reviewer and RimArcho vs the reviewed labels.
//
// Labels, packets and samples hold log text: they stay next to the corpus (corpus-kb\labels),
// never in this repository. Only aggregates go to RESULT.txt.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corpuslabel sample|cooc|packets|validate|score|exec|bench-sample|bench-score|tasks [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	results := fs.String("results", "", "sample: corpusrun results JSONL")
	sampleF := fs.String("sample", "", "the sample JSONL")
	out := fs.String("out", "", "sample: output JSONL; packets: output directory")
	tpl := fs.Int("tpl", 2, "sample: most frequent templates per class")
	per := fs.Int("per", 2, "sample: clusters per template")
	n1 := fs.Int("n1", 20, "bench-sample: clusters per tier-1 class")
	n2 := fs.Int("n2", 15, "bench-sample: clusters per tier-2 class")
	exclude := fs.String("exclude", "", "bench-sample: comma-separated earlier samples whose logs are skipped")
	minCov := fs.Float64("mincov", 0.8, "sample: minimum library coverage of a log")
	seed := fs.String("seed", "p1", "sample: seed of the deterministic pick")
	home := fs.String("home", "", "packets: RimArcho data directory of the research environment")
	corpus := fs.String("corpus", "", "packets: the hugslogs folder (parsed.jsonl, raw/)")
	packets := fs.String("packets", "", "validate/score: packet directory")
	labels := fs.String("labels", "", "validate/score: label directory (one JSON per item)")
	review := fs.String("review", "", "score: reviewer's label directory")
	coocF := fs.String("cooc", "", "packets: co-occurrence JSON from the cooc command (optional)")
	steamF := fs.String("steamdb", "", "packets: RimSort steamDB.json, to name Workshop folders outside the library (optional)")
	gist := fs.String("gist", "", "exec: load this log's mod list, then run the archo command after --")
	packetF := fs.String("packet", "", "exec: take the log from this packet (.txt or .json) instead of -gist")
	logF := fs.String("log", "", "exec: append the command and its output to this transcript")
	execLogs := fs.String("execlogs", "", "validate/bench-score/tasks: directory of exec transcripts <item>.log")
	raF := fs.String("rimarcho", "", "bench-score: rimarcho-v4.jsonl written by bench-sample")
	costsF := fs.String("costs", "", "bench-score: costs.jsonl of the tier-2 runs")
	fs.Parse(os.Args[2:])
	o := &vopts{Corpus: *corpus, ExecLogs: *execLogs}
	var err error
	switch os.Args[1] {
	case "sample":
		err = sample(*results, *out, *tpl, *per, *minCov, *seed)
	case "packets":
		err = buildPackets(*home, *corpus, *sampleF, *out, *coocF, *steamF)
	case "validate":
		err = validateCmd(*sampleF, *packets, *labels, o)
	case "score":
		err = score(*sampleF, *packets, *labels, *review)
	case "cooc":
		err = cooc(*results, *corpus, *sampleF, *out)
	case "exec":
		err = execCmd(*home, *corpus, *gist, *packetF, *logF, fs.Args())
	case "bench-sample":
		err = benchSample(*results, *out, *n1, *n2, *minCov, *seed, strings.Split(*exclude, ","))
	case "bench-score":
		err = benchScore(*sampleF, *packets, *labels, *review, *raF, *execLogs, *costsF, o)
	case "tasks":
		err = exportTasks(*sampleF, *packets, *labels, *review, *out, o)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
