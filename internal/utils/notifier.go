package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// SendServerChan 发送 Server酱通知
func SendServerChan(sendkey string, title string, desp string, verifyTLS bool) bool {
	if sendkey == "" {
		slog.Warn("Server酱 SendKey 未配置")
		return false
	}

	url := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", sendkey)
	maxRetries := 2

	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"desp":  desp,
	})

	for attempt := 0; attempt <= maxRetries; attempt++ {
		client := &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: !verifyTLS},
			},
		}

		resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
		if err != nil {
			if attempt < maxRetries {
				slog.Warn("Server酱通知异常，5秒后重试", "error", err, "attempt", attempt+1)
				time.Sleep(5 * time.Second)
				continue
			}
			slog.Error("Server酱通知失败（已重试）", "error", err)
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 && attempt < maxRetries {
			slog.Warn("Server酱服务端错误，5秒后重试", "status", resp.StatusCode)
			time.Sleep(5 * time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			slog.Warn("Server酱通知发送失败", "status", resp.StatusCode)
			return false
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			slog.Warn("Server酱响应解析失败", "error", err)
			return false
		}

		code, _ := result["code"].(float64)
		if code == 0 {
			slog.Info("Server酱通知发送成功", "title", title)
			return true
		}
		slog.Warn("Server酱通知发送失败", "data", result)
		return false
	}

	return false
}

// NotifyConfig 通知配置结构（供 NotifyCheckinResult 使用）
type NotifyConfig struct {
	Enabled   bool
	Webhook   string
	VerifyTLS bool
}

// NotifyCheckinResult 打卡结果通知
func NotifyCheckinResult(cfg NotifyConfig, actionName string, success bool, message string, timestamp string) {
	if !cfg.Enabled || cfg.Webhook == "" {
		return
	}

	status := "✅ 成功"
	if !success {
		status = "❌ 失败"
	}
	title := fmt.Sprintf("通卡通 %s - %s", status, actionName)
	desp := fmt.Sprintf("**时间**：%s\n\n**结果**：%s", timestamp, message)

	SendServerChan(cfg.Webhook, title, desp, cfg.VerifyTLS)
}
