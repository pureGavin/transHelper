//go:build windows

package win

import (
	"strings"
)

// availableProcesses 算出左边列表：正在运行的程序，加上这次从右边移回来、当前却没在运行的。
// 已经在 enabled 里的不出现。结果按进程名排序，不区分大小写。
func availableProcesses(running, movedBack, enabled []string) []string {
	skip := processSet(enabled)
	seen := make(map[string]struct{})
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := skip[key]; ok {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	for _, name := range running {
		add(name)
	}
	for _, name := range movedBack {
		add(name)
	}
	sortProcessNames(out)
	return out
}

// takeProcess 把 name 从 from 挪到 to 的末尾。比较不区分大小写。
// from 里没有这项时 ok 为 false。to 里已经有同名项时只从 from 删除。
func takeProcess(from, to []string, name string) (newFrom, newTo []string, ok bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return from, to, false
	}
	idx := -1
	picked := ""
	for i, item := range from {
		if strings.EqualFold(strings.TrimSpace(item), key) {
			idx = i
			picked = strings.TrimSpace(item)
			break
		}
	}
	if idx < 0 {
		return from, to, false
	}
	newFrom = append(append([]string(nil), from[:idx]...), from[idx+1:]...)
	if _, exists := processSet(to)[key]; exists {
		return newFrom, append([]string(nil), to...), true
	}
	newTo = append(append([]string(nil), to...), picked)
	return newFrom, newTo, true
}

func sortProcessNames(names []string) {
	for i := 1; i < len(names); i++ {
		item := names[i]
		j := i
		for j > 0 && strings.ToLower(names[j-1]) > strings.ToLower(item) {
			names[j] = names[j-1]
			j--
		}
		names[j] = item
	}
}

func processSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		set[strings.ToLower(name)] = struct{}{}
	}
	return set
}
