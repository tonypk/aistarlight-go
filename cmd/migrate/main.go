package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var (
		dir    = flag.String("dir", "migrations", "migrations directory")
		dbURL  = flag.String("database", os.Getenv("DATABASE_URL"), "database URL")
		action = flag.String("action", "up", "migrate action: up, down, version")
		steps  = flag.Int("steps", 0, "number of steps for down migration")
	)
	flag.Parse()

	if *dbURL == "" {
		slog.Error("database URL required (--database or DATABASE_URL env)")
		os.Exit(1)
	}

	m, err := migrate.New(fmt.Sprintf("file://%s", *dir), *dbURL)
	if err != nil {
		slog.Error("failed to create migrator", "error", err)
		os.Exit(1)
	}
	defer m.Close()

	switch *action {
	case "up":
		err = m.Up()
	case "down":
		if *steps > 0 {
			err = m.Steps(-*steps)
		} else {
			err = m.Steps(-1)
		}
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil {
			slog.Error("failed to get version", "error", verr)
			os.Exit(1)
		}
		fmt.Printf("version: %d, dirty: %v\n", v, dirty)
		return
	default:
		slog.Error("unknown action", "action", *action)
		os.Exit(1)
	}

	if err != nil && err != migrate.ErrNoChange {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	slog.Info("migration completed", "action", *action)
}
