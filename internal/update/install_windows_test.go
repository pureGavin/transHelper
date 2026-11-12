//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSwapRunningExecutable(t *testing.T) {
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, exeName)
	copyFile(t, src, exe)

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "TRANSHELPER_HOLD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	time.Sleep(300 * time.Millisecond)

	newExe := NewExePath(dir)
	if err := os.WriteFile(newExe, []byte("replaced"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Swap(exe, newExe); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "replaced" {
		t.Fatalf("替换后的文件 = %q, %v", data, err)
	}
	if _, err := os.Stat(OldExePath(exe)); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}
