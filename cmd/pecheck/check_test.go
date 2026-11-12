package main

import (
	"debug/pe"
	"strings"
	"testing"
)

func TestProblemsAcceptsHardenedImage(t *testing.T) {
	chars := uint16(pe.IMAGE_DLLCHARACTERISTICS_DYNAMIC_BASE |
		pe.IMAGE_DLLCHARACTERISTICS_HIGH_ENTROPY_VA |
		pe.IMAGE_DLLCHARACTERISTICS_NX_COMPAT)
	if got := problems(chars, 0, []string{".text", ".rdata", ".pdata"}); got != nil {
		t.Fatalf("合格的镜像被拒绝: %v", got)
	}
}

func TestProblemsReportsMissingProtections(t *testing.T) {
	got := problems(0, 3, []string{".text", ".debug_info", "short\x00\x00\x00"})
	text := strings.Join(got, "\n")
	for _, want := range []string{"ASLR", "HIGH_ENTROPY_VA", "DEP", "符号表不为空（3 个）", ".debug_info"} {
		if !strings.Contains(text, want) {
			t.Fatalf("结果缺少 %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "short") {
		t.Fatalf("普通段名不该被当成调试段:\n%s", text)
	}
}
