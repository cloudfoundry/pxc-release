package main

import (
	"log/slog"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	var cfg Config
	if err := ParseConfig(&cfg); err != nil {
		slog.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	if err := NewWorkflow(NewRepository(cfg.MySQL.DB)).Run(cfg.ExcludeUsers...); err != nil {
		slog.Error("failed to configure MySQL audit logging", "error", err)
		os.Exit(1)
	}

	slog.Info("Audit log filter setup completed")
}
