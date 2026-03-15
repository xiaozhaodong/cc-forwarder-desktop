package main

import (
	"context"
	"strings"
	"time"
)

const startupAccountEligibilityTimeout = 2 * time.Second

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
	if a == nil || a.endpointManager == nil {
		return false
	}
	return len(a.endpointManager.GetAllEndpoints()) > 0
}

func (a *App) shouldRunStartupAccountChecks() bool {
	if a == nil || a.accountPoolService == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), startupAccountEligibilityTimeout)
	defer cancel()

	accounts, err := a.accountPoolService.ListAccounts(ctx, true)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("启动连通性检查: 读取账号列表失败", "error", err)
		}
		return false
	}

	for _, account := range accounts {
		if account == nil || !account.Enabled {
			continue
		}
		if strings.TrimSpace(account.CredentialRaw) == "" {
			continue
		}
		return true
	}

	return false
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
	if _, _, err := a.endpointManager.BatchHealthCheckAll(); err != nil && a.logger != nil {
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
	if a.logger != nil {
		a.logger.Debug("启动连通性检查: 账号批量检测待后续任务接入")
	}
}
