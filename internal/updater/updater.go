// Package updater 提供应用更新功能
package updater

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juice4927/tongkatong/internal/utils"
)

// ── 数据结构 ──────────────────────────────────────────────────────

// UpdateAsset 更新资源信息
type UpdateAsset struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	FileName    string `json:"file_name"`
	SHA256     string `json:"sha256"`
	Size        int    `json:"size"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"published_at"`
}

// Manifest 更新清单
type Manifest struct {
	Version string                 `json:"version"`
	Assets  map[string]UpdateAsset `json:"assets"`
	Notes   string                 `json:"notes"`
}

// UpdateState 更新状态
type UpdateState struct {
	Status          string `json:"status"`
	TargetVersion   string `json:"target_version"`
	PreviousVersion string `json:"previous_version"`
	Detail          string `json:"detail"`
	UpdatedAt       string `json:"updated_at"`
}

const (
	UpdateTempDir  = "tongkatong_updates"
	UpdateStateFile = "update_state.json"
	MaxRetries     = 4
	ChunkSize      = 512 * 1024
)

// ── 版本比较 ──────────────────────────────────────────────────────

// VersionGreater 判断 v1 > v2
func VersionGreater(v1, v2 string) bool {
	v1Nums := extractNumbers(v1)
	v2Nums := extractNumbers(v2)
	for i := 0; i < len(v1Nums) && i < len(v2Nums); i++ {
		if v1Nums[i] != v2Nums[i] {
			return v1Nums[i] > v2Nums[i]
		}
	}
	return len(v1Nums) > len(v2Nums)
}

func extractNumbers(v string) []int {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(v, -1)
	nums := make([]int, 0, len(matches))
	for _, m := range matches {
		n, _ := strconv.Atoi(m)
		nums = append(nums, n)
	}
	return nums
}

// ── 清单拉取 ──────────────────────────────────────────────────────

// FetchManifest 拉取更新清单
func FetchManifest(manifestURL string, timeout time.Duration) (*Manifest, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("拉取更新清单失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回 %d", resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("解析更新清单失败: %w", err)
	}

	return &manifest, nil
}

// CheckUpdate 检查更新
func CheckUpdate(manifestURL, currentVersion, edition string) (*UpdateAsset, bool, error) {
	manifest, err := FetchManifest(manifestURL, 15*time.Second)
	if err != nil {
		return nil, false, err
	}

	asset, ok := manifest.Assets[edition]
	if !ok {
		return nil, false, fmt.Errorf("更新清单缺少 %s 版本", edition)
	}

	needsUpdate := VersionGreater(asset.Version, currentVersion)
	return &asset, needsUpdate, nil
}

// ── 下载 ──────────────────────────────────────────────────────────

// ProgressCallback 下载进度回调
type ProgressCallback func(downloaded, total int64)

// DownloadFile 下载文件（支持断点续传 + SHA256 校验）
// 返回下载后文件的 SHA256 十六进制字符串
func DownloadFile(url, destPath string, progressCb ProgressCallback) (string, error) {
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)
	partialPath := destPath + ".part"

	// 检查是否已有部分下载
	var downloaded int64
	if fi, err := os.Stat(partialPath); err == nil {
		downloaded = fi.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	if downloaded > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// 处理 416：部分下载无效，从头重新下载
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		resp.Body.Close()
		downloaded = 0
		// 先删除残缺的 .part 文件，避免残留数据污染
		_ = os.Remove(partialPath)
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		resp, err = client.Do(req)
		if err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()

	var totalSize int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		if idx := strings.LastIndex(contentRange, "/"); idx >= 0 {
			totalSize, _ = strconv.ParseInt(contentRange[idx+1:], 10, 64)
		}
	}
	if totalSize == 0 {
		totalSize = resp.ContentLength + downloaded
	}

	// 打开文件（追加或新建，重启时截断）
	mode := os.O_CREATE | os.O_WRONLY
	if downloaded > 0 && resp.StatusCode == http.StatusPartialContent {
		mode |= os.O_APPEND
	} else if downloaded == 0 {
		mode |= os.O_TRUNC
	}
	file, err := os.OpenFile(partialPath, mode, 0644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 流式计算 SHA256（从头开始校验）
	hasher := sha256.New()
	if downloaded > 0 {
		file.Seek(0, 0)
		if _, err := io.Copy(hasher, file); err != nil {
			return "", fmt.Errorf("计算已下载部分 hash 失败: %w", err)
		}
		file.Seek(0, 2)
	}

	writer := io.MultiWriter(file, hasher)
	buf := make([]byte, ChunkSize)
	written := downloaded

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := writer.Write(buf[:n])
			if writeErr != nil {
				return "", writeErr
			}
			written += int64(n)
			if progressCb != nil {
				progressCb(written, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}

	file.Close()

	// 验证文件大小
	if totalSize > 0 && written != totalSize {
		return "", fmt.Errorf("下载不完整: 预期 %d 字节, 实际 %d 字节", totalSize, written)
	}

	// 重命名为目标文件
	if err := os.Rename(partialPath, destPath); err != nil {
		return "", err
	}

	hashStr := fmt.Sprintf("%x", hasher.Sum(nil))
	return hashStr, nil
}

// SHA256File 计算文件 SHA256
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ── 更新状态管理 ──────────────────────────────────────────────────

// WriteUpdateState 写入更新状态
func WriteUpdateState(baseDir, status, targetVersion, previousVersion, detail string) error {
	state := UpdateState{
		Status:          status,
		TargetVersion:   targetVersion,
		PreviousVersion: previousVersion,
		Detail:          detail,
		UpdatedAt:       time.Now().Format("2006-01-02 15:04:05"),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	statePath := filepath.Join(baseDir, UpdateStateFile)
	return os.WriteFile(statePath, data, 0644)
}

// ReadUpdateState 读取更新状态
func ReadUpdateState(baseDir string) (*UpdateState, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, UpdateStateFile))
	if err != nil {
		return nil, err
	}

	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// ConsumeUpdateState 读取并消费更新状态
func ConsumeUpdateState(baseDir string) *UpdateState {
	state, err := ReadUpdateState(baseDir)
	if err != nil {
		return nil
	}
	// 删除状态文件（消费）
	_ = os.Remove(filepath.Join(baseDir, UpdateStateFile))
	return state
}

// ── 更新启动器 ────────────────────────────────────────────────────

// LaunchUpdater 启动更新器（独立进程）
func LaunchUpdater(updaterExe, downloadedFile, currentExe, targetVersion, currentVersion string, originalArgs []string) error {
	// 检查更新器二进制是否存在
	if _, err := os.Stat(updaterExe); err != nil {
		return fmt.Errorf("更新器不存在: %s", updaterExe)
	}
	stateDir := filepath.Dir(currentExe)
	_ = WriteUpdateState(stateDir, "pending", targetVersion, currentVersion, "Update package downloaded.")

	// 构建参数
	args := []string{
		"--source", downloadedFile,
		"--target", currentExe,
		"--state-dir", stateDir,
		"--version", targetVersion,
		"--prev-version", currentVersion,
	}
	// 透传原始 CLI 参数，以便新版本以相同模式启动
	for _, a := range originalArgs {
		args = append(args, "--passthrough-args", a)
	}

	cmd := exec.Command(updaterExe, args...)
	utils.HideWindow(cmd)
	return cmd.Start()
}
