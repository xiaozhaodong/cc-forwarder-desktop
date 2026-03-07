package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cc-forwarder/config"
	"cc-forwarder/internal/utils"
)

const desktopDefaultConfigFileName = "config.yaml"

func defaultDesktopConfigPath() string {
	return filepath.Join(utils.GetConfigDir(), desktopDefaultConfigFileName)
}

func ensureDesktopConfigFile(configPath string, defaultContent []byte) (created bool, err error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return false, fmt.Errorf("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return false, err
	}
	if _, err := os.Stat(configPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(configPath, defaultContent, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func resolveStartupConfigPath(explicitPath string, explicit bool, configDir string, defaultContent []byte) (string, bool, error) {
	if explicit && strings.TrimSpace(explicitPath) != "" {
		return explicitPath, false, nil
	}
	resolvedPath := filepath.Join(configDir, desktopDefaultConfigFileName)
	created, err := ensureDesktopConfigFile(resolvedPath, defaultContent)
	if err != nil {
		return "", false, err
	}
	return resolvedPath, created, nil
}

func (a *App) resolveStartupConfigPath(logger *slog.Logger) (string, error) {
	resolvedPath, created, err := resolveStartupConfigPath(a.configPath, a.configPathExplicit, utils.GetConfigDir(), defaultConfigContent)
	if err != nil {
		return "", err
	}
	if logger != nil {
		if a.configPathExplicit {
			logger.Info("📝 使用命令行指定配置", "config_file", resolvedPath)
		} else if created {
			logger.Info("📝 已初始化默认配置文件", "config_file", resolvedPath)
		} else {
			logger.Info("📝 使用用户目录配置文件", "config_file", resolvedPath)
		}
	}
	return resolvedPath, nil
}

func (a *App) applyDesktopRuntimePathOverrides(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.Logging.FilePath = filepath.Join(utils.GetLogDir(), "app.log")
	cfg.UsageTracking.DatabasePath = filepath.Join(utils.GetDataDir(), "usage.db")
}
