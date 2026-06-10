package automator

import (
	"log/slog"
	"time"
)

// ── ButtonFinder ───────────────────────────────────────────────────

// ButtonFinder 打卡按钮查找器（三级降级策略）
type ButtonFinder struct{}

// NewButtonFinder 创建按钮查找器
func NewButtonFinder() *ButtonFinder {
	return &ButtonFinder{}
}

// FindAndClickResult 查找并点击结果
type FindAndClickResult struct {
	Clicked  bool
	Message  string
}

// ResolveActionSlot 解析动作所属时段
func ResolveActionSlot(action CheckinAction) (isMorning, isSignin bool, slotLabel string) {
	switch action {
	case MorningSignin:
		return true, true, "上午"
	case MorningSignout:
		return true, false, "上午"
	case AfternoonSignin:
		return false, true, "下午"
	case AfternoonSignout:
		return false, false, "下午"
	}
	return true, true, "未知"
}

// DefaultFindAndClick 三级降级查找并点击打卡按钮
//
//	策略 A：行锚定法 — 找左侧不可点击标签确定行 Y 范围
//	策略 B：全文本扫描 — 直接扫描全部可点击按钮
//	策略 C：兜底方案 — 放宽条件 + 坐标推算
func (bf *ButtonFinder) DefaultFindAndClick(action CheckinAction, actionText string, isMorning, isSignin bool, slotLabel string) FindAndClickResult {
	// 这里需要 device 实例执行 dump_hierarchy 和 click
	// 该方法签名会由具体执行器传入 device 操作函数
	// 此处只保留判断逻辑，实际执行在 UIAutomator2Impl 中
	return FindAndClickResult{Clicked: false, Message: "需 device 执行"}
}

// IsAlreadyCheckedIn 判断该时段是否已打卡（通过 UI 按钮状态判断）
//
//	注意：不能仅靠时间判断，因为时间范围可能与调度窗口重叠。
//	如果 device 参数非空，将通过 XML 解析检查按钮是否已变为时间文本。
//	如果 device 为 nil，返回 false（让执行流程继续，由 Actual verification 决定）。
func IsAlreadyCheckedIn(action CheckinAction, device DeviceOperator) bool {
	if device == nil {
		return false
	}

	// 通过 XML 解析检查按钮状态
	xml, err := device.DumpHierarchy()
	if err != nil {
		return false
	}
	nodes := ParseHierarchyXML(xml)
	w, _, _ := device.WindowSize()
	midX := w / 2

	// 检查目标行是否已变为时间文本（不可点击）
	return VerifyTargetRowTransition(nodes, action, midX)
}

// ── 按钮查找策略 ───────────────────────────────────────────────────

// StrategyARowAnchoring 行锚定法：找左侧不可点击标签确定行 Y 范围
//
//	在考勤页面中，左侧有"签到/签退"标签（不可点击），
//	右侧对应有可点击的时间按钮。通过标签的 bounds 推算按钮位置。
func StrategyARowAnchoring(nodes []UINode, actionText string, midX int) (bool, *Rect) {
	// 1. 找左侧不可点击标签
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == actionText && !isClickable(n.Clickable) {
			r := n.BoundsParsed
			// 左侧标签：x 在屏幕左半部分
			if r.CenterX() < midX {
				return true, r
			}
		}
	}
	return false, nil
}

// StrategyBFullTextScan 全文本扫描：直接找可点击的"签到/签退"按钮
func StrategyBFullTextScan(nodes []UINode, actionText string) bool {
	for _, n := range nodes {
		if n.Text == actionText && isClickable(n.Clickable) {
			return true
		}
	}
	return false
}

func isClickable(s string) bool {
	return s == "true"
}

// CollectRightNodes 收集目标行右侧的时间节点
func CollectRightNodes(nodes []UINode, labelNode *Rect, midX int) []struct {
	Time      string
	Clickable bool
	Rect      *Rect
} {
	var results []struct {
		Time      string
		Clickable bool
		Rect      *Rect
	}

	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		r := n.BoundsParsed
		// 在标签行右侧且垂直重叠
		if r.CenterX() > midX && r.Y1 >= labelNode.Y1-10 && r.Y2 <= labelNode.Y2+10 {
			if IsTimeMatch(n.Text) {
				results = append(results, struct {
					Time      string
					Clickable bool
					Rect      *Rect
				}{
					Time:      n.Text,
					Clickable: isClickable(n.Clickable),
					Rect:      r,
				})
			}
		}
	}
	return results
}

// VerifyTargetRowTransition 检查目标行是否从未点击变为已点击（时间文本替代按钮文本）
func VerifyTargetRowTransition(nodes []UINode, action CheckinAction, midX int) bool {
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
				// 检查右侧是否有时间节点
				rightNodes := CollectRightNodes(nodes, r, midX)
				// 如果有时间节点且都没有 clickable 属性，说明已完成打卡
				if len(rightNodes) > 0 {
					allNotClickable := true
					for _, rn := range rightNodes {
						if rn.Clickable {
							allNotClickable = false
							break
						}
					}
					if allNotClickable {
						slog.Info("检测到目标行已变为完成状态（时间文本不可点击）")
						return true
					}
				}
			}
		}
	}
	return false
}

// FindTimeNearCurrent 找接近当前时间的时间节点
func FindTimeNearCurrent(rightNodes []struct {
	Time      string
	Clickable bool
	Rect      *Rect
}) (string, *Rect, bool) {
	now := time.Now()
	nowMinutes := now.Hour()*60 + now.Minute()

	for _, rn := range rightNodes {
		if !rn.Clickable || !IsTimeMatch(rn.Time) {
			continue
		}
		parts := splitTime(rn.Time)
		if len(parts) != 2 {
			continue
		}
		h, _ := parseInt(parts[0])
		m, _ := parseInt(parts[1])
		nodeMinutes := h*60 + m

		diff := nodeMinutes - nowMinutes
		if diff < 0 {
			diff = -diff
		}
		if diff > 720 {
			diff = 1440 - diff
		}
		if diff <= 3 {
			slog.Info("找到接近当前时间的时间节点", "time", rn.Time, "diff_minutes", diff)
			return rn.Time, rn.Rect, true
		}
	}
	return "", nil, false
}

func splitTime(s string) []string {
	if len(s) != 5 {
		return nil
	}
	return []string{s[:2], s[3:]}
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmtErr("not a digit")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func fmtErr(s string) error {
	return &parseError{s}
}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
