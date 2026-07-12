// Package apppaths 提供跨平台的应用数据目录定位。
//
// 原版（Windows）采用便携模式：数据存放在 exe 同级的 data/ 目录。
// macOS 上可执行文件位于 .app/Contents/MacOS/ 内部，
// 往 bundle 内写入会破坏代码签名，且安装到 /Applications 后无写权限，
// 因此统一改用 ~/Library/Application Support/qssh。
//
// 开发模式（wails3 dev / go run）下，若工作目录存在 data/ 或 bin/data/，
// 则优先使用，便于调试时数据与项目放在一起。
package apppaths

import (
	"os"
	"path/filepath"
	"sync"
)

// appDirName 应用数据目录名
const appDirName = "qssh"

var (
	once    sync.Once
	dataDir string
)

// DataDir 返回应用数据根目录（保证存在）。
// macOS: ~/Library/Application Support/qssh
// 开发模式: ./bin/data 或 ./data（若已存在）
func DataDir() string {
	once.Do(func() {
		dataDir = resolveDataDir()
	})
	return dataDir
}

func resolveDataDir() string {
	// 开发模式：优先使用工作目录下已存在的 data 目录
	if cwd, err := os.Getwd(); err == nil {
		binData := filepath.Join(cwd, "bin", "data")
		if info, err := os.Stat(binData); err == nil && info.IsDir() {
			return binData
		}
		cwdData := filepath.Join(cwd, "data")
		if info, err := os.Stat(cwdData); err == nil && info.IsDir() {
			return cwdData
		}
	}

	// 标准位置：macOS 为 ~/Library/Application Support/qssh
	if base, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(base, appDirName)
		if err := os.MkdirAll(dir, 0700); err == nil {
			return dir
		}
	}

	// 降级：用户主目录下的隐藏目录
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, "."+appDirName)
		if err := os.MkdirAll(dir, 0700); err == nil {
			return dir
		}
	}

	// 最终降级：当前工作目录
	cwd, _ := os.Getwd()
	dir := filepath.Join(cwd, "data")
	os.MkdirAll(dir, 0700)
	return dir
}

// SubDir 返回数据根目录下的子目录（保证存在）。
func SubDir(parts ...string) string {
	dir := filepath.Join(append([]string{DataDir()}, parts...)...)
	os.MkdirAll(dir, 0700)
	return dir
}

// WriteSecure 以 0600 权限写入敏感文件（凭据、密钥、令牌等）。
// 若文件已存在，额外 Chmod 收紧权限，确保旧版本遗留的宽松权限被修正。
func WriteSecure(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	// 修正可能已存在的宽松权限
	os.Chmod(path, 0600)
	return nil
}

