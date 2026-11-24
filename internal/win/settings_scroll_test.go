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

func TestWheelPointOf(t *testing.T) {
	// 常量不能直接转成 uint16，先落到变量上，和投递滚轮时一样。
	off := int16(-32000)
	packed := uintptr(uint16(off)) | uintptr(uint16(int16(40)))<<16
	x, y := wheelPointOf(packed)
	if x != -32000 || y != 40 {
		t.Fatalf("坐标 = (%d, %d)", x, y)
	}
	x, y = wheelPointOf(0)
	if x != 0 || y != 0 {
		t.Fatalf("原点 = (%d, %d)", x, y)
	}
}

func TestPointInRect(t *testing.T) {
	r := rect{Left: 10, Top: 20, Right: 30, Bottom: 40}
	if !pointInRect(10, 20, r) || !pointInRect(29, 39, r) {
		t.Fatal("矩形内的点应该命中，右下边不算在内")
	}
	if pointInRect(30, 20, r) || pointInRect(10, 40, r) || pointInRect(0, 0, r) {
		t.Fatal("矩形外的点不该命中")
	}
}

func TestIntersectRect(t *testing.T) {
	a := rect{Left: 0, Top: 0, Right: 100, Bottom: 50}
	b := rect{Left: 40, Top: 20, Right: 120, Bottom: 80}
	got := intersectRect(a, b)
	if got != (rect{Left: 40, Top: 20, Right: 100, Bottom: 50}) {
		t.Fatalf("交集 = %+v", got)
	}
	if intersectRect(a, rect{Left: 200, Top: 0, Right: 220, Bottom: 10}) != (rect{}) {
		t.Fatal("没有重叠时应为空")
	}
}

func TestListOverflows(t *testing.T) {
	if listOverflows(3, 10, 30) {
		t.Fatal("刚好盛下时不该有滚动条")
	}
	if !listOverflows(4, 10, 30) {
		t.Fatal("多出一行时应该能滚")
	}
	if listOverflows(0, 10, 30) || listOverflows(4, 0, 30) || listOverflows(4, 10, 0) {
		t.Fatal("空列表或高度无效时不该滚")
	}
}

func TestListWheelTarget(t *testing.T) {
	window := rect{Left: 0, Top: 0, Right: 400, Bottom: 300}
	runningBounds := rect{Left: 10, Top: 40, Right: 180, Bottom: 240}
	enabledBounds := rect{Left: 220, Top: 40, Right: 390, Bottom: 240}
	const running, enabled uintptr = 1, 2

	if got := listWheelTarget(30, 80, window, running, enabled, runningBounds, enabledBounds, enabled); got != running {
		t.Fatalf("指针在左列表上应选左列表，得到 %d", got)
	}
	if got := listWheelTarget(250, 80, window, running, enabled, runningBounds, enabledBounds, running); got != enabled {
		t.Fatalf("指针在右列表上应选右列表，得到 %d", got)
	}
	if got := listWheelTarget(200, 80, window, running, enabled, runningBounds, enabledBounds, running); got != 0 {
		t.Fatalf("指针在两个列表之间应滚整页，得到 %d", got)
	}
	if got := listWheelTarget(-32000, -32000, window, running, enabled, runningBounds, enabledBounds, enabled); got != enabled {
		t.Fatalf("指针在窗口外时应按消息收件人，得到 %d", got)
	}
	if got := listWheelTarget(-32000, -32000, window, running, enabled, runningBounds, enabledBounds, 9); got != 0 {
		t.Fatalf("收件人不是列表时应滚整页，得到 %d", got)
	}
	if got := listWheelTarget(30, 80, window, 0, enabled, rect{}, enabledBounds, running); got != 0 {
		t.Fatalf("左列表不存在时不该选中，得到 %d", got)
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
