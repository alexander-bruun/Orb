package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alexander-bruun/orb/services/internal/ingest"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: m4b_chapters -path <m4b>")
		flag.PrintDefaults()
	}
	path := flag.String("path", "", "path to .m4b/.m4a file")
	flag.Parse()
	if *path == "" {
		flag.Usage()
		os.Exit(2)
	}

	info, err := ingest.ProbeM4BForDebug(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "probe error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("duration_ms=%d chapters=%d\n", info.DurationMs, len(info.Chapters))
	for i, ch := range info.Chapters {
		fmt.Printf("%03d start_ms=%d title=%q\n", i+1, ch.StartMs, ch.Title)
	}
}
