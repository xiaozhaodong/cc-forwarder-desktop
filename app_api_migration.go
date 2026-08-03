package main

import "cc-forwarder/internal/migration"

// GetMigrationStatus 返回启动迁移与恢复状态。迁移失败时前端只依赖此只读 API。
func (a *App) GetMigrationStatus() migration.Status {
	if a == nil {
		return migration.Status{State: migration.StartupInitializing, MigrationID: migration.MigrationID}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.migrationCoordinator != nil {
		return a.migrationCoordinator.Status()
	}
	return a.migrationStatus
}

// RetryStartupMigration 仅重试协调器允许的幂等阶段；成功后启动此前被阻断的运行态组件。
func (a *App) RetryStartupMigration() (migration.Status, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.migrationCoordinator == nil {
		return a.migrationStatus, nil
	}
	status, err := a.migrationCoordinator.Run(a.ctx)
	a.migrationStatus = status
	if err != nil {
		return status, err
	}
	if a.isRunning {
		return status, nil
	}
	if err := a.loadRuntimeConfig(a.migrationCoordinator.DatabasePath); err != nil {
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
