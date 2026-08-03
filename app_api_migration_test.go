package main

import (
	"testing"

	"cc-forwarder/internal/migration"
)

func TestGetMigrationStatusWithoutCoordinator(t *testing.T) {
	app := NewApp()
	app.migrationStatus = migration.Status{State: migration.StartupMigrationFailed, Error: "fixture"}
	status := app.GetMigrationStatus()
	if status.State != migration.StartupMigrationFailed || status.Error != "fixture" {
		t.Fatalf("status = %+v", status)
	}
}
