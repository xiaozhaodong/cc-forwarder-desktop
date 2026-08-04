package main

import (
	"testing"
	"time"

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

func TestGetMigrationStatusDoesNotWaitForAppLock(t *testing.T) {
	app := NewApp()
	app.migrationStatus = migration.Status{State: migration.StartupMigrating, Phase: migration.PhasePrepared}

	app.mu.Lock()
	defer app.mu.Unlock()

	done := make(chan migration.Status, 1)
	go func() {
		done <- app.GetMigrationStatus()
	}()

	select {
	case status := <-done:
		if status.State != migration.StartupMigrating || status.Phase != migration.PhasePrepared {
			t.Fatalf("status = %+v", status)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("GetMigrationStatus blocked on the global app lock")
	}
}
