package automator

import (
	"testing"
	"time"
)

func TestCheckinVerifier_HandleConfirmDialog_Success(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="打卡成功" bounds="[0,0][1080,1920]" clickable="false"/>
		</hierarchy>`, nil
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(5)
	if err != nil {
		t.Errorf("expected nil for success dialog, got: %v", err)
	}
}

func TestCheckinVerifier_HandleConfirmDialog_Failure(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="超出距离" bounds="[0,0][1080,1920]" clickable="false"/>
		</hierarchy>`, nil
	}
	mock.clickText = func(text string) bool {
		return true
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(5)
	if err == nil {
		t.Fatal("expected error for failure dialog")
	}
	if err.Error() != "打卡失败(GPS): 超出距离" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestCheckinVerifier_HandleConfirmDialog_ReSignConfirm(t *testing.T) {
	mock := newMockDevice()
	callIndex := 0
	mock.dumpHierarchy = func() (string, error) {
		callIndex++
		if callIndex == 1 {
			// 第一次 dump 返回重新签弹窗
			return `<hierarchy rotation="0">
				<node text="提示" bounds="[200,400][880,800]" clickable="false"/>
				<node text="重新签到" bounds="[200,600][880,700]" clickable="true"/>
			</hierarchy>`, nil
		}
		// 后续返回空
		return `<hierarchy rotation="0"></hierarchy>`, nil
	}
	mock.clickText = func(text string) bool {
		return true
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(5)
	if err != nil {
		t.Errorf("expected nil for re-sign confirm dialog, got: %v", err)
	}
}

func TestCheckinVerifier_HandleConfirmDialog_DumpError(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return "", assertError("dump error")
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(2) // short timeout
	if err != nil {
		t.Errorf("expected nil on dump error (timeout), got: %v", err)
	}
}

func TestCheckinVerifier_DefaultVerify_SuccessKeywords(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{"打卡成功", `<hierarchy rotation="0"><node text="打卡成功" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
		{"签到成功", `<hierarchy rotation="0"><node text="签到成功" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
		{"签退成功", `<hierarchy rotation="0"><node text="签退成功" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockDevice()
			mock.dumpHierarchy = func() (string, error) {
				return tc.xml, nil
			}

			verifier := NewCheckinVerifier(mock, "com.test")
			result := verifier.DefaultVerify(MorningSignin)
			if !result {
				t.Errorf("expected true for %s keyword", tc.name)
			}
		})
	}
}

func TestCheckinVerifier_DefaultVerify_FailKeywords(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{"超出距离", `<hierarchy rotation="0"><node text="超出距离" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
		{"不在打卡范围", `<hierarchy rotation="0"><node text="不在打卡范围" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
		{"签到失败", `<hierarchy rotation="0"><node text="签到失败" bounds="[0,0][1080,1920]" clickable="false"/></hierarchy>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockDevice()
			mock.dumpHierarchy = func() (string, error) {
				return tc.xml, nil
			}
			mock.clickText = func(text string) bool {
				return true
			}

			verifier := NewCheckinVerifier(mock, "com.test")
			result := verifier.DefaultVerify(MorningSignin)
			if result {
				t.Errorf("expected false for %s keyword", tc.name)
			}
		})
	}
}

func TestCheckinVerifier_DefaultVerify_NoMatch(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="随便" bounds="[0,0][1080,1920]" clickable="false"/>
		</hierarchy>`, nil
	}
	mock.windowSize = func() (width, height int, err error) {
		return 1080, 1920, nil
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	result := verifier.DefaultVerify(MorningSignin)
	if result {
		t.Error("expected false when no verification strategy matches")
	}
}

func TestCheckinVerifier_DefaultVerify_DumpError(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return "", assertError("dump error")
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	result := verifier.DefaultVerify(MorningSignin)
	if result {
		t.Error("expected false on dump error")
	}
}

func TestCheckinVerifier_DefaultVerify_RowTransition(t *testing.T) {
	// 模拟"签到"按钮已变为不可点击状态（表示已打卡），右侧有时间节点
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="签到" bounds="[0,800][300,960]" clickable="false"/>
			<node text="08:30" bounds="[700,800][900,960]" clickable="false"/>
		</hierarchy>`, nil
	}
	mock.windowSize = func() (width, height int, err error) {
		return 1080, 1920, nil
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	result := verifier.DefaultVerify(MorningSignin)
	if !result {
		t.Error("expected true when row transition detected (time text present, not clickable)")
	}
}

func TestCheckinVerifier_HandleConfirmDialog_Timeout(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0"></hierarchy>`, nil
	}

	verifier := NewCheckinVerifier(mock, "com.test")

	start := time.Now()
	err := verifier.HandleConfirmDialog(2) // 2 second timeout
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil on timeout, got: %v", err)
	}
	if elapsed > 4*time.Second {
		t.Errorf("HandleConfirmDialog took too long: %v", elapsed)
	}
}

func TestCheckinVerifier_HandleConfirmDialog_CancelKeywords(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="退出登录" bounds="[200,400][880,800]" clickable="true"/>
			<node text="提示" bounds="[200,300][880,400]" clickable="false"/>
		</hierarchy>`, nil
	}
	mock.clickText = func(text string) bool {
		return text == "取消"
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(5)
	if err != nil {
		t.Errorf("expected nil for cancel dialog, got: %v", err)
	}
}

func TestCheckinVerifier_HandleConfirmDialog_ReSignWithCancel(t *testing.T) {
	mock := newMockDevice()
	callIndex := 0
	mock.dumpHierarchy = func() (string, error) {
		callIndex++
		if callIndex == 1 {
			return `<hierarchy rotation="0">
				<node text="提示" bounds="[200,400][880,800]" clickable="false"/>
				<node text="删除" bounds="[200,600][880,700]" clickable="true"/>
			</hierarchy>`, nil
		}
		return `<hierarchy rotation="0"></hierarchy>`, nil
	}
	mock.clickText = func(text string) bool {
		return text == "取消"
	}

	verifier := NewCheckinVerifier(mock, "com.test")
	err := verifier.HandleConfirmDialog(5)
	if err != nil {
		t.Errorf("expected nil when cancel keyword detected, got: %v", err)
	}
}
