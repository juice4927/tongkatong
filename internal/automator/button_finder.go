package automator

import (
	"log/slog"
	"strconv"
	"strings"
	"time"
)

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

// IsAlreadyCheckedIn 判断该时段是否已打卡（通过 UI 按钮状态判断）
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

// ── 按钮查找策略 ───────────────────────────────────────────────────

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
		if diff <= 3 {
			slog.Info("找到接近当前时间的时间节点", "time", rn.Time, "diff_minutes", diff)
			return rn.Time, rn.Rect, true
		}
	}
	return "", nil, false
}

