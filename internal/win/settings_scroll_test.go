//go:build windows

package win

import "testing"

func TestClampScroll(t *testing.T) {
	checks := []struct {
		pos, content, view, want int
	}{
		{0, 100, 200, 0},
		{40, 100, 200, 0},
		{-5, 500, 100, 0},
		{10, 500, 100, 10},
		{450, 500, 100, 400},
		{0, 100, 100, 0},
		{1, 100, 100, 0},
		{3, 50, 0, 3},
		{-1, 50, -4, 0},
	}
	for _, check := range checks {
		if got := clampScroll(check.pos, check.content, check.view); got != check.want {
			t.Errorf("clampScroll(%d, %d, %d) = %d，期望 %d", check.pos, check.content, check.view, got, check.want)
		}
	}
}

func TestAdjustHitTest(t *testing.T) {
	if adjustHitTest(htLeft) != htBorder || adjustHitTest(htRight) != htBorder {
		t.Fatal("左右边不应该能拖")
	}
	if adjustHitTest(htTopLeft) != htTop || adjustHitTest(htTopRight) != htTop {
		t.Fatal("上角应该改成上边")
	}
	if adjustHitTest(htBottomLeft) != htBottom || adjustHitTest(htBottomRight) != htBottom {
		t.Fatal("下角应该改成下边")
	}
	if adjustHitTest(htTop) != htTop || adjustHitTest(htBottom) != htBottom || adjustHitTest(1) != 1 {
		t.Fatal("上下边和客户区应该保持原样")
	}
}

func TestSettingsHeightBounds(t *testing.T) {
	s := &settingsWindow{dpi: 96}
	_, natural := s.clientSize()
	minH := s.minClientHeight()
	if minH >= natural {
		t.Fatalf("最小高度 %d 应该小于完整高度 %d", minH, natural)
	}
	// 最小高度要容得下标签、两行内容和按钮。
	if minH < int32(layoutTabH+2*layoutRowH+layoutBtnH) {
		t.Fatalf("最小高度过小: %d", minH)
	}
}
