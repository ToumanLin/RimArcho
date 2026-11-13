// lanzou-probe diagnoses one download without installing or executing its bytes.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/updatefeed"
)

type prober func(context.Context, string, string, string, []string, func(updatefeed.ProbeEvent)) (updatefeed.ProbeResult, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, func(ctx context.Context, share, password, filename string, hosts []string, observe func(updatefeed.ProbeEvent)) (updatefeed.ProbeResult, error) {
		return updatefeed.ProbeLanzou(ctx, nil, share, password, filename, hosts, observe)
	}))
}

func run(ctx context.Context, args []string, out io.Writer, probe prober) int {
	flags := flag.NewFlagSet("lanzou-probe", flag.ContinueOnError)
	flags.SetOutput(out)
	share := flags.String("url", "", "Lanzou folder share URL")
	password := flags.String("password", "", "folder password")
	filename := flags.String("filename", "", "exact ZIP filename")
	hosts := flags.String("hosts", "", "additional exact allowed hosts, comma-separated (no wildcards)")
	// flag errors echo user input, which could contain a password or bearer URL.
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		flags.SetOutput(out)
		if err != flag.ErrHelp {
			fmt.Fprintln(out, "Invalid arguments. Supply -url, -password and -filename.")
		}
		flags.PrintDefaults()
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *share == "" || *filename == "" || flags.NArg() != 0 {
		fmt.Fprintln(out, "Supply -url and -filename; use -password for a protected folder.")
		return 2
	}
	var extra []string
	if *hosts != "" {
		extra = strings.Split(*hosts, ",")
	}
	fmt.Fprintln(out, "Lanzou probe: temporary download only; no install or execution.")
	result, err := probe(ctx, *share, *password, *filename, extra, func(event updatefeed.ProbeEvent) {
		fmt.Fprintf(out, "stage=%s", event.Stage)
		if event.Host != "" {
			fmt.Fprintf(out, " host=%s", event.Host)
		}
		if event.Status != 0 {
			fmt.Fprintf(out, " status=%d", event.Status)
		}
		fmt.Fprintln(out)
	})
	if err != nil {
		fmt.Fprintf(out, "FAILED: %v\nlast_host=%s bytes=unavailable sha256=unavailable\n", err, result.Host)
		return 1
	}
	fmt.Fprintf(out, "final_host=%s bytes=%d sha256=%s\nTemporary download deleted. SHA-256 is an observation, not release authentication.\n", result.Host, result.Size, result.SHA256)
	return 0
}
