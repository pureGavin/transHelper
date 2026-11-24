//go:build windows

package win

import "testing"

func TestAcceptableModelURL(t *testing.T) {
	ok := []string{
		"http://localhost:8000/v1",
		"https://localhost:8000/v1/",
		"http://127.0.0.1:9/v1",
	}
	for _, raw := range ok {
		if !acceptableModelURL(raw) {
			t.Fatalf("%q 应该可用", raw)
		}
	}
	bad := []string{"", "localhost:8000/v1", "ftp://localhost/v1", "http://", "/v1"}
	for _, raw := range bad {
		if acceptableModelURL(raw) {
			t.Fatalf("%q 不该可用", raw)
		}
	}
}

func TestGeneralBodyFits(t *testing.T) {
	if generalBody() >= processBody() || generalBody() >= networkBody() {
		t.Fatalf("通用页高度 %d 不应超过进程页 %d 或网络页 %d", generalBody(), processBody(), networkBody())
	}
}
