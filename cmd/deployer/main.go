package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/Bhup-GitHUB/cell-based-arch/internal/deploy"
)

func main() {
	version := flag.String("version", "", "app version to roll out (e.g. v2) — required")
	rollbackTo := flag.String("rollback-to", "v1", "version to restore on canary failure")
	bake := flag.Duration("bake", 5*time.Second, "wait after recreating a cell before running the canary")
	canaryTimeout := flag.Duration("canary-timeout", 30*time.Second, "overall timeout for a single cell's canary check")
	canarySuccesses := flag.Int("canary-successes", 3, "consecutive 200 responses required to pass canary")
	composeFile := flag.String("compose-file", "infra/docker-compose.yml", "path to docker-compose file")
	failCell := flag.Int("fail-cell", 0, "if >0 inject a fault on that cell number to simulate a bad deploy")

	flag.Parse()

	if *version == "" {
		slog.Error("--version is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg := deploy.Config{
		Version:         *version,
		RollbackTo:      *rollbackTo,
		Bake:            *bake,
		CanaryTimeout:   *canaryTimeout,
		CanarySuccesses: *canarySuccesses,
		ComposeFile:     *composeFile,
		FailCell:        *failCell,
		Cells:           deploy.DefaultCells(),
	}

	slog.Info("deployer starting",
		"version", cfg.Version,
		"rollback_to", cfg.RollbackTo,
		"bake", cfg.Bake,
		"canary_timeout", cfg.CanaryTimeout,
		"canary_successes", cfg.CanarySuccesses,
		"compose_file", cfg.ComposeFile,
		"fail_cell", cfg.FailCell,
	)

	if err := deploy.Run(cfg); err != nil {
		slog.Error("deployer failed", "err", err)
		os.Exit(1)
	}
}
