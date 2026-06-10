package automator

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── 已打卡时间范围常量 ────────────────────────────────────────────

// AlreadyCheckedInRanges 4 个时段的已打卡判定范围
var AlreadyCheckedInRanges = map[CheckinAction]struct{ StartHour, StartMin, EndHour, EndMin int }{
	MorningSignin:    {7, 0, 9, 0},
	MorningSignout:   {11, 0, 13, 0},
	AfternoonSignin:  {12, 0, 14, 0},
	AfternoonSignout: {17, 0, 28, 0}, // 跨天
}

// ── 错误类型 ──────────────────────────────────────────────────────

// AlreadyCheckedInError 已打卡错误（区分正确时段 vs 异常弹窗）
type AlreadyCheckedInError struct {
	Message       string
	InCorrectSlot bool   // true=正确时段内已打卡, false=异常弹窗显示已打卡
	CheckinTime   string // 打卡时间
}

func (e *AlreadyCheckedInError) Error() string { return e.Message }

// GpsLocationError GPS 定位错误
type GpsLocationError struct {
	Message     string
	FailureCode string
}

func (e *GpsLocationError) Error() string { return e.Message }

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

// IsAlreadyCheckedIn 判断该时段是否已打卡
func IsAlreadyCheckedIn(action CheckinAction, device DeviceOperator) bool {
	if device == nil {
		return false
	}
	xml, err := device.DumpHierarchy()
	if err != nil {
		return false
	}
	nodes := ParseHierarchyXML(xml)
	w, _, _ := device.WindowSize()
	midX := w / 2
	return VerifyTargetRowTransition(nodes, action, midX)
}

func isClickable(s string) bool { return s == "true" }

// ── 按钮查找三策略 ─────────────────────────────────────────────────

// FindAndClickButton 三策略按钮查找+点击
// 策略 A: 行锚定法 — 左侧标签 → 右侧时间按钮
// 策略 B: 全文本扫描 — 按 Y 排序，选上午/下午对应行
// 策略 C: 兜底 — 文本包含匹配 + 放宽条件
// 返回: (是否成功, error)
func FindAndClickButton(action CheckinAction, device DeviceOperator) (bool, error) {
	if device == nil {
		return false, fmt.Errorf("device is nil")
	}
	xml, err := device.DumpHierarchy()
	if err != nil {
		return false, fmt.Errorf("dump hierarchy: %w", err)
	}

	nodes := ParseHierarchyXML(xml)
	w, _, _ := device.WindowSize()
	midX := int(float64(w) * 0.5)

	_, isSignin, _ := ResolveActionSlot(action)
	actionText := "签到"
	if !isSignin {
		actionText = "签退"
	}

	// 策略 A：行锚定法 — 找左侧标签，点击右侧可点击时间节点
	for _, n := range nodes {
		if n.BoundsParsed == nil || n.Text != actionText || isClickable(n.Clickable) {
			continue
		}
		r := n.BoundsParsed
		if r.CenterX() >= midX {
			continue
		}

		rightNodes := CollectRightNodes(nodes, r, midX)
		for _, rn := range rightNodes {
			if !rn.Clickable || rn.Rect == nil {
				continue
			}
			// 检查是否已打卡（时间不可点击=已完成）
			allDone := true
			for _, rn2 := range rightNodes {
				if rn2.Clickable {
					allDone = false
					break
				}
			}
			if allDone && len(rightNodes) > 0 {
				return false, &AlreadyCheckedInError{
					Message:       fmt.Sprintf("%s 该时段已打卡", actionText),
					InCorrectSlot: true,
					CheckinTime:   rightNodes[0].Time,
				}
			}

			slog.Info("策略A·行锚定：点击右侧时间", "time", rn.Time)
			if err := device.Click(rn.Rect.CenterX(), rn.Rect.CenterY()); err == nil {
				return true, nil
			}
		}
	}

	// 策略 B：全文本扫描 — 按 Y 排序，上午选上半部分，下午选下半部分
	slog.Info("策略B·全文本扫描", "text", actionText)
	isMorning, _, _ := ResolveActionSlot(action)

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].BoundsParsed == nil {
			return false
		}
		if nodes[j].BoundsParsed == nil {
			return true
		}
		return nodes[i].BoundsParsed.Y1 < nodes[j].BoundsParsed.Y1
	})

	var candidates []UINode
	for _, n := range nodes {
		if n.BoundsParsed == nil || !isClickable(n.Clickable) {
			continue
		}
		if strings.Contains(n.Text, actionText) {
			candidates = append(candidates, n)
		}
	}

	for _, c := range candidates {
		y := c.BoundsParsed.Y1
		_, h, _ := device.WindowSize()
		if (isMorning && y < h/2) || (!isMorning && y >= h/2) {
			if err := device.Click(c.BoundsParsed.CenterX(), c.BoundsParsed.CenterY()); err == nil {
				slog.Info("策略B·全文本扫描：点击匹配", "text", c.Text, "y", y)
				return true, nil
			}
		}
	}

	// 策略 C：兜底 — 文本包含匹配任何可点击元素
	slog.Info("策略C·兜底方案", "text", actionText)
	for _, n := range nodes {
		if n.BoundsParsed == nil || !isClickable(n.Clickable) {
			continue
		}
		if strings.Contains(n.Text, actionText) {
			if err := device.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY()); err == nil {
				slog.Info("策略C·包含匹配：点击", "text", n.Text)
				return true, nil
			}
		}
	}

	// 放宽：不要求 clickable
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == actionText || strings.Contains(n.Text, actionText) {
			if err := device.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY()); err == nil {
				slog.Info("策略C·放宽条件：点击", "text", n.Text)
				time.Sleep(500 * time.Millisecond)
				return true, nil
			}
		}
	}

	return false, fmt.Errorf("未找到 '%s' 按钮", actionText)
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

// VerifyTargetRowTransition 检查目标行是否已打卡
func VerifyTargetRowTransition(nodes []UINode, action CheckinAction, midX int) bool {
	_, isSignin, _ := ResolveActionSlot(action)
	actionText := "签到"
	if !isSignin {
		actionText = "签退"
	}

	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == actionText && !isClickable(n.Clickable) {
			r := n.BoundsParsed
			if r.CenterX() < midX {
				rightNodes := CollectRightNodes(nodes, r, midX)
				if len(rightNodes) > 0 {
					allNotClickable := true
					for _, rn := range rightNodes {
						if rn.Clickable {
							allNotClickable = false
							break
						}
					}
					if allNotClickable {
						slog.Info("检测到目标行已变为完成状态")
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
		parts := strings.Split(rn.Time, ":")
		if len(parts) != 2 {
			continue
		}
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		nodeMinutes := h*60 + m
		diff := nodeMinutes - nowMinutes
		if diff < 0 {
			diff = -diff
		}
		if diff > 720 {
			diff = 1440 - diff
		}
		if diff <= 5 {
			slog.Info("找到接近当前时间的时间节点", "time", rn.Time, "diff_minutes", diff)
			return rn.Time, rn.Rect, true
		}
	}
	return "", nil, false
}
