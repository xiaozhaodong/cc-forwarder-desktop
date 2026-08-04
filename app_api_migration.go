package main

import "cc-forwarder/internal/migration"

// GetMigrationStatus 返回启动迁移与恢复状态。迁移失败时前端只依赖此只读 API。
func (a *App) GetMigrationStatus() migration.Status {
	if a == nil {
		return migration.Status{State: migration.StartupInitializing, MigrationID: migration.MigrationID}
	}
	coordinator, status := a.migrationState()
	if coordinator != nil {
		return coordinator.Status()
	}
	return status
}

// RetryStartupMigration 仅重试协调器允许的幂等阶段；成功后启动此前被阻断的运行态组件。
func (a *App) RetryStartupMigration() (migration.Status, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	coordinator, fallback := a.migrationState()
	if coordinator == nil {
		return fallback, nil
	}
	status, err := coordinator.Run(a.ctx)
	a.setMigrationStatus(status)
	if err != nil {
		return status, err
	}
	if a.isRunning {
		return status, nil
	}
	if err := a.loadRuntimeConfig(coordinator.DatabasePath); err != nil {
		return status, err
	}
	if a.coreDatabase != nil {
		if err := a.coreDatabase.InitSchema(); err != nil {
			return status, err
		}
	}
	a.setupLogger()
	a.startOperationalComponentsLocked(a.ctx)
	return status, nil
}

func (a *App) migrationState() (*migration.Coordinator, migration.Status) {
	a.migrationMu.RLock()
	defer a.migrationMu.RUnlock()
	return a.migrationCoordinator, a.migrationStatus
}

func (a *App) setMigrationCoordinator(coordinator *migration.Coordinator) {
	a.migrationMu.Lock()
	a.migrationCoordinator = coordinator
	a.migrationMu.Unlock()
}

func (a *App) setMigrationStatus(status migration.Status) {
	a.migrationMu.Lock()
	a.migrationStatus = status
	a.migrationMu.Unlock()
}
