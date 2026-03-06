package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"lean-context-mode/internal/lean"
)

const version = "0.1.0"

func defaultRootFromEnv() string {
	if v := os.Getenv("LCM_ROOT"); v != "" {
		return v
	}
	if v := os.Getenv("LEAN_CONTEXT_MODE_ROOT"); v != "" {
		return v
	}
	return "."
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "stats":
			runStats(os.Args[2:])
			return
		case "serve":
			runServe(os.Args[2:])
			return
		}
	}
	runServe(os.Args[1:])
}

func runStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	root := fs.String("root", defaultRootFromEnv(), "workspace root")
	_ = fs.Parse(args)

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve root:", err)
		os.Exit(1)
	}
	metrics, err := lean.NewMetricsStore(abs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open metrics:", err)
		os.Exit(1)
	}
	fmt.Print(metrics.StatsText())
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	root := fs.String("root", defaultRootFromEnv(), "workspace root")
	_ = fs.Parse(args)

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve root:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rm, err := lean.NewRootManager(ctx, abs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "init root manager:", err)
		os.Exit(1)
	}
	defer rm.Stop()

	if err := lean.RunMCPServerDynamic(ctx, rm, version); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
