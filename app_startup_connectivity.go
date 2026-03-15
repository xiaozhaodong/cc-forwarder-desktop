package main

func (a *App) scheduleStartupConnectivityChecks() {
	if a == nil {
		return
	}
	if a.shouldRunStartupEndpointChecks() {
		go a.runStartupEndpointChecks()
	}
	if a.shouldRunStartupAccountChecks() {
		go a.runStartupAccountChecks()
	}
}

func (a *App) shouldRunStartupEndpointChecks() bool {
	if a == nil {
		return false
	}
	if a.endpointManager == nil {
		return false
	}
	return len(a.endpointManager.GetAllEndpoints()) > 0
}

func (a *App) shouldRunStartupAccountChecks() bool {
	if a == nil {
		return false
	}
	if a.startupAccountCheckRunner != nil {
		return true
	}
	return a.accountPoolService != nil
}

func (a *App) runStartupEndpointChecks() {
	if a == nil {
		return
	}
	if a.startupEndpointCheckRunner != nil {
		a.startupEndpointCheckRunner()
		return
	}
	if a.endpointManager == nil {
		return
	}
	_, _, err := a.endpointManager.BatchHealthCheckAll()
	if err != nil && a.logger != nil {
		a.logger.Warn("启动连通性检查: 端点批量检测失败", "error", err)
	}
}

func (a *App) runStartupAccountChecks() {
	if a == nil {
		return
	}
	if a.startupAccountCheckRunner != nil {
		a.startupAccountCheckRunner()
		return
	}
	if a.accountPoolService == nil {
		return
	}

	summary := a.accountPoolService.RunStartupConnectivityChecks(a.ctx, 0)
	if summary.Total == 0 {
		if a.logger != nil {
			a.logger.Debug(
				"启动连通性检查: 无可测账号，跳过账号批量检测",
				"skipped", summary.SkippedCount,
			)
		}
		return
	}

	if a.logger != nil {
		a.logger.Info(
			"启动连通性检查: 账号批量检测完成",
			"total", summary.Total,
			"success", summary.SuccessCount,
			"failed", summary.FailureCount,
			"skipped", summary.SkippedCount,
		)
	}
}
