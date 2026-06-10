package automator

import (
	"log/slog"
	"slices"
	"time"
)

var (
	DefaultCancelRatio  = [2]float64{0.30, 0.594}
	DefaultConfirmRatio = [2]float64{0.70, 0.594}
)

// DialogHandler 通用弹窗处理
type DialogHandler struct {
	device       DeviceOperator
	cancelRatio  [2]float64
	confirmRatio [2]float64
}

func NewDialogHandler(device DeviceOperator) *DialogHandler {
	return &DialogHandler{
		device:       device,
		cancelRatio:  DefaultCancelRatio,
		confirmRatio: DefaultConfirmRatio,
	}
}

func (h *DialogHandler) ClickButtonByText(texts []string) bool {
	screenW, screenH, err := h.device.WindowSize()
	if err != nil {
		screenW, screenH = 1080, 1920
	}

	for _, text := range texts {
		if h.device.ClickText(text) {
			slog.Info("找到原生按钮并点击", "text", text)
			time.Sleep(1 * time.Second)
			return true
		}
	}

	if slices.Contains(texts, "取消") {
		if found := h.clickByMirror("确定", screenW, true); found {
			return true
		}
		if found := h.clickByMirror("确认", screenW, true); found {
			return true
		}
	}

	if slices.Contains(texts, "确定") || slices.Contains(texts, "确认") || slices.Contains(texts, "OK") {
		if found := h.clickByMirror("取消", screenW, false); found {
			return true
		}
	}

	isCancel := slices.Contains(texts, "取消") || slices.Contains(texts, "关闭") || slices.Contains(texts, "返回")
	ratio := h.confirmRatio
	if isCancel {
		ratio = h.cancelRatio
	}
	cx := int(float64(screenW) * ratio[0])
	cy := int(float64(screenH) * ratio[1])
	slog.Info("按比例点击按钮", "x", cx, "y", cy)
	if err := h.device.Click(cx, cy); err == nil {
		time.Sleep(1 * time.Second)
		return true
	}
	return false
}

func (h *DialogHandler) clickByMirror(knowText string, screenW int, _ bool) bool {
	xml, err := h.device.DumpHierarchy()
	if err != nil {
		return false
	}
	nodes := ParseHierarchyXML(xml)
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == knowText {
			r := n.BoundsParsed
			targetX := screenW - r.CenterX()
			targetY := r.CenterY()
			slog.Info("镜像法推算按钮位置", "known", knowText)
			if err := h.device.Click(targetX, targetY); err == nil {
				time.Sleep(1 * time.Second)
				return true
			}
			return false
		}
	}
	return false
}
