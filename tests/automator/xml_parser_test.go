package automator_test

import (
	"testing"

	"github.com/juice4927/tongkatong/internal/automator"
)

func TestXMLParser(t *testing.T) {
	xml := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" class="android.widget.FrameLayout" package="com.test"
    bounds="[0,0][1080,1920]">
    <node index="1" text="签到" class="android.widget.Button" package="com.test"
      clickable="true" bounds="[540,800][1080,960]"/>
    <node index="2" text="签退" class="android.widget.TextView" package="com.test"
      clickable="false" bounds="[0,800][540,960]"/>
    <node index="3" text="08:30" class="android.widget.TextView" package="com.test"
      clickable="true" bounds="[540,800][1080,960]"/>
  </node>
</hierarchy>`

	nodes := automator.ParseHierarchyXML(xml)

	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}

	foundSignin := false
	foundSignout := false
	for _, n := range nodes {
		if n.Text == "签到" && n.Clickable == "true" {
			foundSignin = true
		}
		if n.Text == "签退" && n.Clickable == "false" {
			foundSignout = true
		}
	}
	if !foundSignin {
		t.Error("expected clickable '签到' button")
	}
	if !foundSignout {
		t.Error("expected non-clickable '签退' label")
	}

	// Test bounds parsing
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			t.Errorf("node '%s' has no bounds", n.Text)
			continue
		}
		if n.Text == "签到" {
			if n.BoundsParsed.CenterX() != 810 {
				t.Errorf("签到 centerX should be 810, got %d", n.BoundsParsed.CenterX())
			}
		}
	}
}

func TestResolveActionSlot(t *testing.T) {
	tests := []struct {
		action    automator.CheckinAction
		isMorning bool
		isSignin  bool
		label     string
	}{
		{automator.MorningSignin, true, true, "上午"},
		{automator.MorningSignout, true, false, "上午"},
		{automator.AfternoonSignin, false, true, "下午"},
		{automator.AfternoonSignout, false, false, "下午"},
	}

	for _, tc := range tests {
		isMg, isSi, label := automator.ResolveActionSlot(tc.action)
		if isMg != tc.isMorning || isSi != tc.isSignin || label != tc.label {
			t.Errorf("%v: got (%v,%v,%s), want (%v,%v,%s)",
				tc.action, isMg, isSi, label, tc.isMorning, tc.isSignin, tc.label)
		}
	}
}

func TestIsTimeMatch(t *testing.T) {
	if !automator.IsTimeMatch("08:30") {
		t.Error("08:30 should match")
	}
	if automator.IsTimeMatch("25:00") {
		t.Error("25:00 should not match")
	}
	if automator.IsTimeMatch("abc") {
		t.Error("abc should not match")
	}
}
