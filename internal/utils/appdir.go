package utils

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAppDataDir 获取应用数据目录（跨平台）
// Windows: %APPDATA%\AI-Switchboard
// macOS: ~/Library/Application Support/AI-Switchboard
// Linux: ~/.local/share/ai-switchboard
func GetAppDataDir() string {
	return resolveAppDataDir("AI-Switchboard", "ai-switchboard", ".ai-switchboard")
}

// GetLegacyAppDataDir 获取品牌更名前（CC-Forwarder 时代）的数据目录，仅用于迁移
func GetLegacyAppDataDir() string {
	return resolveAppDataDir("CC-Forwarder", "cc-forwarder", ".cc-forwarder")
}

func resolveAppDataDir(desktopName, unixName, fallbackName string) string {
	switch runtime.GOOS {
	case "windows":
		baseDir := os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(baseDir, desktopName)

	case "darwin":
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "Library", "Application Support", desktopName)

	case "linux":
		if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
			return filepath.Join(xdgDataHome, unixName)
		}
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".local", "share", unixName)

	default:
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, fallbackName)
	}
}

// MigrateLegacyAppDataDir 将旧品牌数据目录一次性迁移到新目录。
// 仅当旧目录存在且新目录不存在时整体 rename（新旧目录同处一个父目录，操作原子）；
// 新目录已存在时不做任何改动，避免覆盖。返回是否发生了迁移。
func MigrateLegacyAppDataDir() (bool, error) {
	oldDir := GetLegacyAppDataDir()
	newDir := GetAppDataDir()
	if oldDir == newDir {
		return false, nil
	}
	if _, err := os.Stat(oldDir); err != nil {
		return false, nil
	}
	if _, err := os.Stat(newDir); err == nil {
		return false, nil
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return false, err
	}
	return true, nil
}

// GetDataDir 获取数据库目录
func GetDataDir() string {
	return filepath.Join(GetAppDataDir(), "data")
}

// GetLogDir 获取日志目录
func GetLogDir() string {
	return filepath.Join(GetAppDataDir(), "logs")
}

// GetConfigDir 获取配置目录
func GetConfigDir() string {
	return filepath.Join(GetAppDataDir(), "config")
}

// EnsureAppDirs 确保应用所需的所有目录存在
func EnsureAppDirs() error {
	dirs := []string{
		GetAppDataDir(),
		GetDataDir(),
		GetLogDir(),
		GetConfigDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
