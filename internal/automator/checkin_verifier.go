package automator

import (
	"log/slog"
	"time"

	"github.com/juice4927/tongkatong/internal/models"
)

// ── 打卡验证模块 ────────────────────────────────────────────────────

// CheckinVerifier 打卡结果验证器
type CheckinVerifier struct {
	device        DeviceOperator
	packageName   string
	dialogHandler *DialogHandler
}

// NewCheckinVerifier 创建打卡验证器
func NewCheckinVerifier(device DeviceOperator, packageName string) *CheckinVerifier {
	return &CheckinVerifier{
		device:        device,
		packageName:   packageName,
		dialogHandler: NewDialogHandler(device),
	}
}

// HandleConfirmDialog 处理覆盖打卡时的确认对话框
func (v *CheckinVerifier) HandleConfirmDialog(timeout int) error {
	start := time.Now()

	for time.Since(start) < time.Duration(timeout)*time.Second {
		time.Sleep(500 * time.Millisecond)

		xml, err := v.device.DumpHierarchy()
		if err != nil {
			continue
		}

		// 检测失败弹窗
		failKeywords := []string{"超出距离", "不在打卡范围", "定位失败", "网络异常", "打卡失败"}
		for _, kw := range failKeywords {
			if containsSubstring(xml, kw) {
				slog.Warn("检测到打卡失败弹窗", "keyword", kw)
				// 关闭弹窗
				v.dialogHandler.ClickButtonByText([]string{"确定", "知道了", "关闭"})
				// 失败弹窗返回错误
				return &CheckinError{Message: "打卡失败: " + kw, FailureCode: string(models.OutsideRange)}
			}
		}

		// 检测成功弹窗 — 只记录成功状态，不关闭弹窗（留给 DefaultVerify 再次确认）
		successKeywords := []string{"打卡成功", "签到成功", "签退成功"}
		for _, kw := range successKeywords {
			if containsSubstring(xml, kw) {
				slog.Info("检测到成功弹窗", "keyword", kw)
				// 先关闭弹窗让后续流程继续
				v.dialogHandler.ClickButtonByText([]string{"确定", "知道了", "关闭"})
				// 设置标记，让 DefaultVerify 知道已完成
				time.Sleep(1 * time.Second)
				return nil
			}
		}

		// 检测需要确认的弹窗
		if containsSubstring(xml, "重新签") || containsSubstring(xml, `text="提示"`) {
			cancelKeywords := []string{"退出登录", "退出", "注销", "删除", "移除"}
			shouldCancel := false
			for _, kw := range cancelKeywords {
				if containsSubstring(xml, kw) {
					shouldCancel = true
					break
				}
			}

			if shouldCancel {
				slog.Info("检测到需要取消的弹窗")
				v.dialogHandler.ClickButtonByText([]string{"取消", "关闭", "返回"})
				return nil
			}

			// 确认覆盖打卡
			slog.Info("检测到覆盖确认弹窗")
			v.dialogHandler.ClickButtonByText([]string{"确定", "确认", "OK"})
			return nil
		}

		// 无弹窗特征持续 8 秒后退出
		if time.Since(start) > 8*time.Second {
			slog.Debug("未检测到弹窗特征，提前退出")
			return nil
		}
	}

	slog.Debug("等待弹窗超时")
	return nil
}

// DefaultVerify 默认验证打卡是否成功
//
//	策略：
//	 1. 检测失败弹窗关键词 → 直接返回 false
//	 2. 检测成功弹窗关键词 → 直接返回 true
//	 3. 检测目标行从按钮变为时间/完成态 → 返回 true
//	 4. 检测右侧可点击时间节点接近当前时间 → 返回 true
//	 5. 以上均未命中 → 返回 false
func (v *CheckinVerifier) DefaultVerify(action CheckinAction) bool {
	xml, err := v.device.DumpHierarchy()
	if err != nil {
		slog.Warn("验证打卡时获取 XML 失败", "error", err)
		return false
	}

	// 1. 检测失败弹窗
	failKeywords := []string{
		"超出距离", "不在打卡范围", "定位失败", "网络异常", "打卡失败",
		"请先定位", "无法获取", "签到失败", "签退失败",
	}
	for _, kw := range failKeywords {
		if containsSubstring(xml, kw) {
			slog.Warn("检测到失败提示", "keyword", kw)
			v.dialogHandler.ClickButtonByText([]string{"确定", "知道了", "关闭", "取消"})
			return false
		}
	}

	// 2. 检测成功弹窗
	successTexts := []string{"打卡成功", "签到成功", "签退成功"}
	for _, text := range successTexts {
		if containsSubstring(xml, text) {
			return true
		}
	}

	// 3. 解析 XML 并检查状态变化
	nodes := ParseHierarchyXML(xml)
	w, _, _ := v.device.WindowSize()
	midX := int(float64(w) * 0.5)

	// 检查目标行状态变化
	if VerifyTargetRowTransition(nodes, action, midX) {
		return true
	}

	// 4. 检查右侧可点击时间节点
	_, isSignin, _ := ResolveActionSlot(action)
	actionText := "签到"
	if !isSignin {
		actionText = "签退"
	}

	// 找左侧标签
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == actionText && !isClickable(n.Clickable) {
			r := n.BoundsParsed
			if r.CenterX() < midX {
				rightNodes := CollectRightNodes(nodes, r, midX)
				if len(rightNodes) == 0 {
					slog.Warn("未定位到目标行右侧状态，判定打卡失败")
					return false
				}

				// 检查是否有接近当前时间的时间节点
				if _, _, found := FindTimeNearCurrent(rightNodes); found {
					return true
				}

				slog.Warn("未检测到成功标志，判定打卡失败")
				return false
			}
		}
	}

	slog.Warn("未找到目标行，判定打卡失败")
	return false
}

// ── 助手函数 ───────────────────────────────────────────────────────

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) != -1
}

func searchString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// CheckinError 打卡错误
type CheckinError struct {
	Message     string
	FailureCode string
}

func (e *CheckinError) Error() string {
	return e.Message
}
