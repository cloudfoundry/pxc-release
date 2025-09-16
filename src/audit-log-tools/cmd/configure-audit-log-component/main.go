package main

import (
	"log/slog"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"auditlogtools"
)

func main() {
	handlerOptions := &slog.HandlerOptions{Level: slog.LevelDebug}
	handler := slog.NewJSONHandler(os.Stderr, handlerOptions)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	var cfg Config
	if err := ParseConfig(&cfg); err != nil {
		slog.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	repository := auditlogtools.NewRepository(cfg.MySQL.DB)
	if err := auditlogtools.NewWorkflow(repository).Run(auditlogtools.WorkflowOptions{
		ExcludeUsers:  cfg.ExcludeUsers,
		DefaultFilter: cfg.DefaultFilter,
	}); err != nil {
		slog.Error("failed to configure MySQL audit logging", "error", err)
		os.Exit(1)
	}

	slog.Info("Audit log filter setup completed")
}
