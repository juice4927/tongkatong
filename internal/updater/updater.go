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
	UpdateTempDir   = "tongkatong_updates"
	UpdateStateFile = "update_state.json"
	MaxRetries      = 4
	ChunkSize       = 512 * 1024
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

// DownloadFile 下载文件（支持断点续传 + 流式 SHA256 校验）
func DownloadFile(url, destPath string, progressCb ProgressCallback) error {
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)
	partialPath := destPath + ".part"

	// 检查是否已有部分下载
	var downloaded int64
	if fi, err := os.Stat(partialPath); err == nil {
		downloaded = fi.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if downloaded > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// 416 表示文件已完整，直接进入校验
		downloaded = 0
		resp.Body.Close()
		resp, err = client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}

	var totalSize int64
	// 尝试从 Content-Range 或 Content-Length 获取总大小
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		if idx := strings.LastIndex(contentRange, "/"); idx >= 0 {
			totalSize, _ = strconv.ParseInt(contentRange[idx+1:], 10, 64)
		}
	}
	if totalSize == 0 {
		totalSize = resp.ContentLength + downloaded
	}

	// 打开文件（追加或新建）
	mode := os.O_CREATE | os.O_WRONLY
	if downloaded > 0 && resp.StatusCode == http.StatusPartialContent {
		mode |= os.O_APPEND
	}
	file, err := os.OpenFile(partialPath, mode, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 流式计算 SHA256（从头开始校验）
	hasher := sha256.New()
	if downloaded > 0 {
		// 已有部分，先计算已下载部分的 hash
		file.Seek(0, 0)
		io.Copy(hasher, file)
		file.Seek(0, 2) // 回到末尾
	}

	// 同时写入文件和 hash 计算
	writer := io.MultiWriter(file, hasher)
	buf := make([]byte, ChunkSize)
	written := downloaded

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			wn, writeErr := writer.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if wn != n {
				return io.ErrShortWrite
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
			return readErr
		}
	}

	// 关闭文件以刷新
	file.Close()

	// 重命名为目标文件
	if err := os.Rename(partialPath, destPath); err != nil {
		return err
	}

	return nil
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
func LaunchUpdater(updaterExe, downloadedFile, currentExe, targetVersion, currentVersion string) error {
	stateDir := filepath.Dir(currentExe)

	// 写入状态
	_ = WriteUpdateState(stateDir, "pending", targetVersion, currentVersion, "Update package downloaded.")

	// 构建参数
	args := []string{
		"--source", downloadedFile,
		"--target", currentExe,
		"--state-dir", stateDir,
		"--version", targetVersion,
		"--prev-version", currentVersion,
	}

	cmd := exec.Command(updaterExe, args...)
	utils.HideWindow(cmd)
	return cmd.Start()
}

// ── 节假日数据热更新 ──────────────────────────────────────────────

// HolidayData 节假日数据
type HolidayData struct {
	Holidays    map[string]string `json:"holidays"`
	InLieuDays map[string]string `json:"in_lieu_days"`
}

// FetchHolidayData 从远程拉取节假日数据
func FetchHolidayData(url string) (*HolidayData, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data HolidayData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SaveHolidayCache 保存节假日数据到本地缓存
func SaveHolidayCache(cachePath string, data *HolidayData) error {
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return os.WriteFile(cachePath, jsonData, 0644)
}

// LoadHolidayCache 从本地缓存加载节假日数据
func LoadHolidayCache(cachePath string) (*HolidayData, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var hd HolidayData
	if err := json.Unmarshal(data, &hd); err != nil {
		return nil, err
	}
	return &hd, nil
}
