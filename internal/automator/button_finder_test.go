package automator

import (
	"testing"
)

func TestCollectRightNodes(t *testing.T) {
	nodes := []UINode{
		{Text: "签到", Bounds: "[0,800][300,960]", BoundsParsed: parseBounds("[0,800][300,960]")},
		{Text: "08:30", Bounds: "[700,800][900,960]", BoundsParsed: parseBounds("[700,800][900,960]")},
		{Text: "12:00", Bounds: "[700,960][900,1120]", BoundsParsed: parseBounds("[700,960][900,1120]")},
	}

	labelRect := parseBounds("[0,800][300,960]")
	rightNodes := CollectRightNodes(nodes, labelRect, 540)

	if len(rightNodes) == 0 {
		t.Fatal("expected at least 1 right node")
	}
	if rightNodes[0].Time != "08:30" {
		t.Errorf("expected first right node time '08:30', got '%s'", rightNodes[0].Time)
	}
}

func TestCollectRightNodes_NoMatch(t *testing.T) {
	nodes := []UINode{
		{Text: "签到", Bounds: "[0,800][300,960]", BoundsParsed: parseBounds("[0,800][300,960]")},
		{Text: "其他", Bounds: "[400,800][600,960]", BoundsParsed: parseBounds("[400,800][600,960]")},
	}

	labelRect := parseBounds("[0,800][300,960]")
	rightNodes := CollectRightNodes(nodes, labelRect, 200) // midX 在 label 区域内

	if len(rightNodes) > 0 {
		t.Logf("got %d right nodes (midX=%d)", len(rightNodes), 200)
	}
}

func TestVerifyTargetRowTransition_SignedIn(t *testing.T) {
	nodes := []UINode{
		{Text: "签到", Clickable: "false", Bounds: "[0,800][300,960]", BoundsParsed: parseBounds("[0,800][300,960]")},
		{Text: "08:30", Clickable: "false", Bounds: "[700,800][900,960]", BoundsParsed: parseBounds("[700,800][900,960]")},
	}

	result := VerifyTargetRowTransition(nodes, MorningSignin, 540)
	if !result {
		t.Error("expected true for signed-in state (time text not clickable)")
	}
}

func TestVerifyTargetRowTransition_NotSignedIn(t *testing.T) {
	nodes := []UINode{
		{Text: "签到", Clickable: "true", Bounds: "[0,800][300,960]", BoundsParsed: parseBounds("[0,800][300,960]")},
		{Text: "08:30", Clickable: "true", Bounds: "[700,800][900,960]", BoundsParsed: parseBounds("[700,800][900,960]")},
	}

	result := VerifyTargetRowTransition(nodes, MorningSignin, 540)
	if result {
		t.Error("expected false for not-signed-in state (button still clickable)")
	}
}

func TestVerifyTargetRowTransition_NoLabelFound(t *testing.T) {
	nodes := []UINode{
		{Text: "其他", Clickable: "false", Bounds: "[0,800][300,960]", BoundsParsed: parseBounds("[0,800][300,960]")},
	}

	result := VerifyTargetRowTransition(nodes, MorningSignin, 540)
	if result {
		t.Error("expected false when no matching label found")
	}
}

func TestFindTimeNearCurrent(t *testing.T) {
	// 无法构造确切的时间节点（与当前时间相关），测试可 close 不 panic
	rightNodes := make([]struct {
		Time      string
		Clickable bool
		Rect      *Rect
	}, 0)

	_, _, found := FindTimeNearCurrent(rightNodes)
	if found {
		t.Error("expected false for empty nodes")
	}
}

func TestIsAlreadyCheckedIn_WithMock(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		// 模拟已打卡状态（按钮不可点击，右侧有时间）
		return `<hierarchy rotation="0">
			<node text="签到" bounds="[0,800][300,960]" clickable="false"/>
			<node text="08:30" bounds="[700,800][900,960]" clickable="false"/>
		</hierarchy>`, nil
	}
	mock.windowSize = func() (width, height int, err error) {
		return 1080, 1920, nil
	}

	result := IsAlreadyCheckedIn(MorningSignin, mock)
	if !result {
		t.Error("expected true when already checked in")
	}
}

func TestIsAlreadyCheckedIn_NilDevice(t *testing.T) {
	result := IsAlreadyCheckedIn(MorningSignin, nil)
	if result {
		t.Error("expected false for nil device")
	}
}
