// Command migrate applies SQL migrations to the database.
package main

import (
	"fmt"
	"os"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/config"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/storage/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Applying database migrations to: %s\n", cfg.Database.URL)
	if err := postgres.RunMigrations(cfg.Database.URL, cfg.Database.MigrationsPath); err != nil {
		fmt.Printf("Migration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("SUCCESS: All database migrations applied successfully!")
}
