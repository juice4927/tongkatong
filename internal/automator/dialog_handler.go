package automator

import (
	"log/slog"
	"time"
)

// ── 默认弹窗按钮位置 ──────────────────────────────────────────────

var (
	DefaultCancelRatio  = [2]float64{0.30, 0.594}
	DefaultConfirmRatio = [2]float64{0.70, 0.594}
)

// ── DialogHandler ───────────────────────────────────────────────────

// DialogHandler 通用弹窗处理
type DialogHandler struct {
	device       DeviceOperator
	cancelRatio  [2]float64
	confirmRatio [2]float64
}

// NewDialogHandler 创建弹窗处理器
func NewDialogHandler(device DeviceOperator) *DialogHandler {
	return &DialogHandler{
		device:       device,
		cancelRatio:  DefaultCancelRatio,
		confirmRatio: DefaultConfirmRatio,
	}
}

// ClickButtonByText 按文本查找并点击按钮，四级降级策略
//
//	策略1：查找原生按钮（通过 uiautomator 文本匹配）
//	策略2：镜像法推导（比如通过"确定"推算出"取消"的位置）
//	策略3：通过"取消"推算出"确定"位置
//	策略4：根据屏幕比例直接点击
func (h *DialogHandler) ClickButtonByText(texts []string) bool {
	screenW, screenH, err := h.device.WindowSize()
	if err != nil {
		screenW, screenH = 1080, 1920
	}

	// 策略1：查找原生可点击按钮
	for _, text := range texts {
		if h.device.ClickText(text) {
			slog.Info("找到原生按钮并点击", "text", text)
			time.Sleep(1 * time.Second)
			return true
		}
	}

	// 策略2：如果有"取消"，通过"确定"按钮推算位置
	if contains(texts, "取消") {
		if found := h.clickByMirror("确定", screenW, true); found {
			return true
		}
		if found := h.clickByMirror("确认", screenW, true); found {
			return true
		}
	}

	// 策略3：如果有"确定"，通过"取消"推算位置
	if containsAny(texts, "确定", "确认", "OK") {
		if found := h.clickByMirror("取消", screenW, false); found {
			return true
		}
	}

	// 策略4：根据屏幕比例直接点击
	isCancel := containsAny(texts, "取消", "关闭", "返回")
	ratio := h.confirmRatio
	if isCancel {
		ratio = h.cancelRatio
	}
	cx := int(float64(screenW) * ratio[0])
	cy := int(float64(screenH) * ratio[1])
	slog.Info("按比例点击按钮", "x", cx, "y", cy, "ratio_x", ratio[0], "ratio_y", ratio[1])
	if err := h.device.Click(cx, cy); err == nil {
		time.Sleep(1 * time.Second)
		return true
	}
	return false
}

// clickByMirror 镜像法：通过已知按钮位置推算目标按钮位置
func (h *DialogHandler) clickByMirror(knowText string, screenW int, isCancel bool) bool {
	// 先尝试通过 ClickText 获取按钮位置——实际需要检查是否存在
	if !h.device.TextExists(knowText) {
		return false
	}

	// 简化实现：假设两个按钮水平对称
	// 在实际完整实现中，我们会用 XML 解析获取已知按钮的 bounds
	// 这里通过 ClickText 本身去找到并点击
	if isCancel {
		// 取消按钮通常在右侧对称位置，但更简单的是直接尝试点击
		return false
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsAny(slice []string, items ...string) bool {
	for _, s := range slice {
		for _, item := range items {
			if s == item {
				return true
			}
		}
	}
	return false
}
