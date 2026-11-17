//go:build windows

package win

import (
	"reflect"
	"testing"
)

func TestAvailableProcesses(t *testing.T) {
	got := availableProcesses(
		[]string{"Cursor.exe", "Weixin.exe", "cursor.exe"},
		[]string{"Teams.exe", "Weixin.exe"},
		[]string{"ms-teams.exe"},
	)
	want := []string{"Cursor.exe", "Teams.exe", "Weixin.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("左边 = %#v，期望 %#v", got, want)
	}

	kept := availableProcesses([]string{"Notepad.exe"}, []string{"Teams.exe"}, []string{"notepad.exe"})
	if len(kept) != 1 || kept[0] != "Teams.exe" {
		t.Fatalf("已启用的不该留在左边: %#v", kept)
	}
}

func TestTakeProcess(t *testing.T) {
	from, to, ok := takeProcess([]string{"A.exe", "B.exe"}, []string{"C.exe"}, "b.exe")
	if !ok || !reflect.DeepEqual(from, []string{"A.exe"}) || !reflect.DeepEqual(to, []string{"C.exe", "B.exe"}) {
		t.Fatalf("移动结果 from=%#v to=%#v ok=%v", from, to, ok)
	}

	from, to, ok = takeProcess([]string{"A.exe"}, []string{"a.exe"}, "A.exe")
	if !ok || len(from) != 0 || !reflect.DeepEqual(to, []string{"a.exe"}) {
		t.Fatalf("重复项不该再加一次: from=%#v to=%#v ok=%v", from, to, ok)
	}

	from, to, ok = takeProcess([]string{"A.exe"}, nil, "missing.exe")
	if ok || !reflect.DeepEqual(from, []string{"A.exe"}) || to != nil {
		t.Fatalf("找不到时不该改动: from=%#v to=%#v ok=%v", from, to, ok)
	}
}
