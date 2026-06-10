// tongkatong-build - 通卡通构建与发布工具
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	projectRoot string
	outputDir   string
	version     string
)

func main() {
	flag.StringVar(&version, "version", "", "版本号（如 3.0.0）")
	flag.Parse()

	var err error
	projectRoot, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工作目录失败: %v\n", err)
		os.Exit(1)
	}

	outputDir = filepath.Join(projectRoot, "build_out")
	_ = os.MkdirAll(outputDir, 0755)

	if version == "" {
		version = readCurrentVersion()
	}
	fmt.Printf("通卡通构建工具 v%s\n", version)

	// 1. 构建主程序
	fmt.Println("构建 tongkatong.exe...")
	mainExe := filepath.Join(outputDir, fmt.Sprintf("tongkatong_v%s.exe", version))
	if err := buildExe(mainExe); err != nil {
		fmt.Fprintf(os.Stderr, "构建失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  输出: %s\n", mainExe)

	// 2. 构建更新器
	fmt.Println("构建更新器...")
	updaterExe := filepath.Join(outputDir, "tongkatong-updater.exe")
	if err := buildUpdater(updaterExe); err != nil {
		fmt.Fprintf(os.Stderr, "更新器构建失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  输出: %s\n", updaterExe)

	// 3. 生成更新清单
	fmt.Println("生成更新清单...")
	if err := generateManifest(version, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "生成更新清单失败: %v\n", err)
	}

	fmt.Println("构建完成!")
}

func buildExe(output string) error {
	args := []string{"build", "-ldflags", "-s -w", "-o", output, "./cmd/tongkatong/"}
	cmd := exec.Command("go", args...)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildUpdater(output string) error {
	cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", output, "./cmd/updater/")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readCurrentVersion() string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "internal", "models", "version.go"))
	if err != nil {
		return "3.0.0"
	}
	content := string(data)
	start := strings.Index(content, `Version = "`)
	if start < 0 {
		return "3.0.0"
	}
	start += len(`Version = "`)
	end := strings.Index(content[start:], `"`)
	if end < 0 {
		return "3.0.0"
	}
	return content[start : start+end]
}

func generateManifest(ver, distDir string) error {
	owner := os.Getenv("GITHUB_REPOSITORY_OWNER")
	if owner == "" {
		owner = "juice4927"
	}

	fileName := fmt.Sprintf("tongkatong_v%s.exe", ver)
	exePath := filepath.Join(distDir, fileName)

	// 计算 SHA256 和文件大小
	var sha256Str string
	var fileSize int64
	if fi, err := os.Stat(exePath); err == nil {
		fileSize = fi.Size()
		if f, err := os.Open(exePath); err == nil {
			h := sha256.New()
			io.Copy(h, f)
			f.Close()
			sha256Str = fmt.Sprintf("%x", h.Sum(nil))
		}
	}

	manifest := map[string]interface{}{
		"version": ver,
		"assets": map[string]interface{}{
			"default": map[string]interface{}{
				"version":      ver,
				"file_name":    fileName,
				"url":          fmt.Sprintf("https://github.com/%s/tongkatong/releases/download/v%s/%s", owner, ver, fileName),
				"sha256":       sha256Str,
				"size":         fileSize,
				"published_at": time.Now().Format("2006-01-02"),
			},
		},
		"notes":        fmt.Sprintf("通卡通 v%s 发布", ver),
		"published_at": time.Now().Format("2006-01-02"),
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(distDir, "version.json")
	_ = os.WriteFile(manifestPath, data, 0644)
	fmt.Printf("  更新清单: %s\n", manifestPath)
	return nil
}
