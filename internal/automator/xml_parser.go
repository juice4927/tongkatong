// Package automator 提供自动化打卡核心引擎
package automator

import (
	"encoding/xml"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
)

// ── 正则常量 ──────────────────────────────────────────────────────

var (
	// 匹配 HH:MM 格式，00:00-23:59
	timePattern   = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	invalidXML    = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]`)
)

// UINode UI 节点信息
type UINode struct {
	Text           string `xml:"text,attr"`
	ResourceID     string `xml:"resource-id,attr"`
	Class          string `xml:"class,attr"`
	ContentDesc    string `xml:"content-desc,attr"`
	Checkable      string `xml:"checkable,attr"`
	Checked        string `xml:"checked,attr"`
	Clickable      string `xml:"clickable,attr"`
	Enabled        string `xml:"enabled,attr"`
	Bounds         string `xml:"bounds,attr"`
	Package        string `xml:"package,attr"`
	BoundsParsed   *Rect  `xml:"-"`
}

// Rect 矩形区域
type Rect struct {
	X1, Y1, X2, Y2 int
}

// CenterX 中心点 X
func (r *Rect) CenterX() int { return (r.X1 + r.X2) / 2 }

// CenterY 中心点 Y
func (r *Rect) CenterY() int { return (r.Y1 + r.Y2) / 2 }

// Width 宽度
func (r *Rect) Width() int { return r.X2 - r.X1 }

// Height 高度
func (r *Rect) Height() int { return r.Y2 - r.Y1 }

// Contains 判断点是否在矩形内
func (r *Rect) Contains(x, y int) bool {
	return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2
}

// ── XML 层级解析 ───────────────────────────────────────────────────

// HierarchyXML UI hierarchy XML 结构
type HierarchyXML struct {
	XMLName xml.Name   `xml:"hierarchy"`
	Nodes   []xmlNode  `xml:"node"`
}

type xmlNode struct {
	XMLName   xml.Name  `xml:"node"`
	Text      string    `xml:"text,attr"`
	ResourceID string   `xml:"resource-id,attr"`
	Class     string    `xml:"class,attr"`
	ContentDesc string  `xml:"content-desc,attr"`
	Checkable string    `xml:"checkable,attr"`
	Checked   string    `xml:"checked,attr"`
	Clickable string    `xml:"clickable,attr"`
	Enabled   string    `xml:"enabled,attr"`
	Bounds    string    `xml:"bounds,attr"`
	Package   string    `xml:"package,attr"`
	Nodes     []xmlNode `xml:"node"`
}

// ParseHierarchyXML 解析 uiautomator dump 的 hierarchy XML
func ParseHierarchyXML(xmlStr string) []UINode {
	// 清理非法字符
	cleaned := invalidXML.ReplaceAllString(xmlStr, "")
	cleaned = strings.TrimSpace(cleaned)

	var root HierarchyXML
	if err := xml.Unmarshal([]byte(cleaned), &root); err != nil {
		slog.Warn("XML 解析失败（ET），回退正则", "error", err)
		return parseWithRegex(cleaned)
	}

	var nodes []UINode
	flattenNodes(root.Nodes, &nodes)

	// 解析 bounds
	for i := range nodes {
		nodes[i].BoundsParsed = parseBounds(nodes[i].Bounds)
	}

	return nodes
}

func flattenNodes(xmlNodes []xmlNode, result *[]UINode) {
	for _, n := range xmlNodes {
		*result = append(*result, UINode{
			Text:        n.Text,
			ResourceID:  n.ResourceID,
			Class:       n.Class,
			ContentDesc: n.ContentDesc,
			Checkable:   n.Checkable,
			Checked:     n.Checked,
			Clickable:   n.Clickable,
			Enabled:     n.Enabled,
			Bounds:      n.Bounds,
			Package:     n.Package,
		})
		flattenNodes(n.Nodes, result)
	}
}

// parseWithRegex 正则回退解析
func parseWithRegex(xmlStr string) []UINode {
	var nodes []UINode
	re := regexp.MustCompile(`<node\b([^>]*/?>)`)
	matches := re.FindAllStringSubmatch(xmlStr, -1)

	for _, m := range matches {
		attrs := m[1]
		node := UINode{
			Text:        extractAttr(attrs, `text="([^"]*)"`),
			ResourceID:  extractAttr(attrs, `resource-id="([^"]*)"`),
			Class:       extractAttr(attrs, `class="([^"]*)"`),
			ContentDesc: extractAttr(attrs, `content-desc="([^"]*)"`),
			Checkable:   extractAttr(attrs, `checkable="([^"]*)"`),
			Checked:     extractAttr(attrs, `checked="([^"]*)"`),
			Clickable:   extractAttr(attrs, `clickable="([^"]*)"`),
			Enabled:     extractAttr(attrs, `enabled="([^"]*)"`),
			Bounds:      extractAttr(attrs, `bounds="([^"]*)"`),
			Package:     extractAttr(attrs, `package="([^"]*)"`),
		}
		node.BoundsParsed = parseBounds(node.Bounds)
		nodes = append(nodes, node)
	}
	return nodes
}

func extractAttr(input, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(input)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// parseBounds 解析 bounds 字符串 "[0,0][1080,1920]" → Rect
func parseBounds(bounds string) *Rect {
	// 去掉可能的 bounds= 前缀
	if idx := strings.Index(bounds, "["); idx >= 0 {
		bounds = bounds[idx:]
	}
	// 匹配 [x1,y1][x2,y2]
	re := regexp.MustCompile(`^\[(\d+),(\d+)\]\[(\d+),(\d+)\]$`)
	m := re.FindStringSubmatch(bounds)
	if len(m) != 5 {
		return nil
	}
	x1, _ := strconv.Atoi(m[1])
	y1, _ := strconv.Atoi(m[2])
	x2, _ := strconv.Atoi(m[3])
	y2, _ := strconv.Atoi(m[4])
	return &Rect{X1: x1, Y1: y1, X2: x2, Y2: y2}
}

// ExtractCenterDialogTexts 从 XML 中提取屏幕中部弹窗区域的文本
func ExtractCenterDialogTexts(xmlStr string, screenW, screenH int) string {
	nodes := ParseHierarchyXML(xmlStr)
	var texts []string
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		// 弹窗通常位于屏幕中央 20%-80% 区域
		centerX1 := int(float64(screenW) * 0.2)
		centerX2 := int(float64(screenW) * 0.8)
		centerY1 := int(float64(screenH) * 0.2)
		centerY2 := int(float64(screenH) * 0.8)

		r := n.BoundsParsed
		if r.X1 >= centerX1 && r.X2 <= centerX2 && r.Y1 >= centerY1 && r.Y2 <= centerY2 {
			if n.Text != "" {
				texts = append(texts, n.Text)
			}
		}
	}
	return strings.Join(texts, " ")
}

// IsTimeMatch 检查字符串是否匹配 HH:MM 格式
func IsTimeMatch(s string) bool {
	return timePattern.MatchString(s)
}
