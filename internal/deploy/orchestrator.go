package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

type CellConfig struct {
	Number     int
	Service    string
	HealthURL  string
	VersionEnv string
	FailEnv    string
}

type Config struct {
	Version         string
	RollbackTo      string
	Bake            time.Duration
	CanaryTimeout   time.Duration
	CanarySuccesses int
	ComposeFile     string
	FailCell        int
	Cells           []CellConfig
}

func DefaultCells() []CellConfig {
	return []CellConfig{
		{
			Number:     1,
			Service:    "cell1-app",
			HealthURL:  "http://localhost:9001/healthz",
			VersionEnv: "APP_VERSION_1",
			FailEnv:    "CELL_FAIL_1",
		},
		{
			Number:     2,
			Service:    "cell2-app",
			HealthURL:  "http://localhost:9002/healthz",
			VersionEnv: "APP_VERSION_2",
			FailEnv:    "CELL_FAIL_2",
		},
		{
			Number:     3,
			Service:    "cell3-app",
			HealthURL:  "http://localhost:9003/healthz",
			VersionEnv: "APP_VERSION_3",
			FailEnv:    "CELL_FAIL_3",
		},
	}
}

func composeBase() []string {
	if exec.Command("docker", "compose", "version").Run() == nil {
		return []string{"docker", "compose"}
	}
	return []string{"docker-compose"}
}

func composeUp(ctx context.Context, composeFile, service string, extraEnv []string) error {
	args := append(composeBase(), "-f", composeFile, "up", "-d", "--no-deps", "--build", service)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Run(cfg Config) error {
	promoted := make([]string, 0, len(cfg.Cells))

	for _, cell := range cfg.Cells {
		slog.Info("promoting", "service", cell.Service, "version", cfg.Version)

		promoteEnv := []string{
			fmt.Sprintf("%s=%s", cell.VersionEnv, cfg.Version),
			fmt.Sprintf("%s=false", cell.FailEnv),
		}
		if cfg.FailCell == cell.Number {
			slog.Warn("fault injection active", "service", cell.Service, "cell", cell.Number)
			promoteEnv = append(promoteEnv, fmt.Sprintf("%s=true", cell.FailEnv))
		}

		promoteCtx, promoteCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err := composeUp(promoteCtx, cfg.ComposeFile, cell.Service, promoteEnv)
		promoteCancel()
		if err != nil {
			return fmt.Errorf("compose up failed for %s: %w", cell.Service, err)
		}

		slog.Info("baking", "service", cell.Service, "duration", cfg.Bake)
		time.Sleep(cfg.Bake)

		slog.Info("running canary", "service", cell.Service, "url", cell.HealthURL)
		canaryCtx, canaryCancel := context.WithTimeout(context.Background(), cfg.CanaryTimeout)
		canaryErr := RunCanary(canaryCtx, cell.Service, cell.HealthURL, cfg.CanarySuccesses, 2*time.Second)
		canaryCancel()

		if canaryErr == nil {
			slog.Info("cell promoted successfully", "service", cell.Service, "version", cfg.Version)
			promoted = append(promoted, cell.Service)
			continue
		}

		slog.Error("canary failed", "service", cell.Service, "err", canaryErr)
		slog.Info("rolling back", "service", cell.Service, "rollback_version", cfg.RollbackTo)

		rollbackEnv := []string{
			fmt.Sprintf("%s=%s", cell.VersionEnv, cfg.RollbackTo),
			fmt.Sprintf("%s=false", cell.FailEnv),
		}

		rbCtx, rbCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		rbErr := composeUp(rbCtx, cfg.ComposeFile, cell.Service, rollbackEnv)
		rbCancel()
		if rbErr != nil {
			slog.Error("rollback compose up failed", "service", cell.Service, "err", rbErr)
		} else {
			slog.Info("rollback compose up complete; baking before confirming recovery", "service", cell.Service, "duration", cfg.Bake)
			time.Sleep(cfg.Bake)

			recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), cfg.CanaryTimeout)
			recoveryErr := RunCanary(recoveryCtx, cell.Service, cell.HealthURL, cfg.CanarySuccesses, 2*time.Second)
			recoveryCancel()
			if recoveryErr == nil {
				slog.Info("rollback confirmed healthy", "service", cell.Service, "version", cfg.RollbackTo)
			} else {
				slog.Error("rollback canary also failed — cell may be degraded", "service", cell.Service, "err", recoveryErr)
			}
		}

		slog.Info("rollout summary",
			"promoted", promoted,
			"failed_cell", cell.Service,
			"rolled_back_to", cfg.RollbackTo,
			"remaining_cells_untouched", remainingServices(cfg.Cells, cell.Number),
		)
		return fmt.Errorf("rollout stopped: canary failed on %s", cell.Service)
	}

	slog.Info("rollout complete", "promoted", promoted, "version", cfg.Version)
	return nil
}

func remainingServices(cells []CellConfig, failedNumber int) []string {
	var names []string
	for _, c := range cells {
		if c.Number > failedNumber {
			names = append(names, c.Service)
		}
	}
	return names
}
