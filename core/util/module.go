package util

import (
	"os"
	"path/filepath"
	"strings"
)

func GetModuleNameAndModuleRootDir() (moduleRootDir string, moduleName string, err error) {
	moduleName, err = GetModuleName()
	if err != nil {
		return "", "", err
	}
	moduleRootDir, err = GetModuleRootDir()
	if err != nil {
		return "", "", err
	}
	return
}

func GetModuleName() (string, error) {
	rootDir, err := GetModuleRootDir()
	if err != nil {
		return "", err
	}

	modPath := filepath.Join(rootDir, "go.mod")
	modBytes, err := os.ReadFile(modPath)
	if err != nil {
		return "", err
	}

	modLine := strings.TrimPrefix(strings.Split(string(modBytes), "\n")[0], "module ")
	return strings.TrimSpace(modLine), nil
}

func GetModuleRootDir() (string, error) {
	dir, err := os.Getwd() // 获取当前工作目录
	if err != nil {
		return "", err
	}

	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			return dir, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir { // 到达文件系统根目录
			break
		}
		dir = parentDir
	}
	return "", os.ErrNotExist // 未找到 go.mod
}
