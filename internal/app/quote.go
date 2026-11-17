package app

import (
	"regexp"
	"strings"
)

const quoteLookahead = 8

// onWrotePattern 匹配 “On Mon, Nov 17, 2026 at 4:00 PM Bob wrote:” 这种整行。
var onWrotePattern = regexp.MustCompile(`(?i)^on\s+.+\s+wrote[：:]\s*$`)

// zaiWrotePattern 匹配 “在 … 写道：” 和 “于 … 写道：”。于 是网页邮件里 On 的常见译法。
var zaiWrotePattern = regexp.MustCompile(`^(?:在|于)\s*.+\s*写道[：:]\s*$`)

// hasMailQuote 报告文本里是否有邮件引用头。
// 以 > 开头的行不会单独算作引用，聊天里的引用格式因此不会被误判。
func hasMailQuote(text string) bool {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if isMailSeparator(line) || isWroteLine(line) {
			return true
		}
		if isHeader(line, "from", "发件人") && hasHeaderAhead(lines, i, "sent", "date", "发送时间", "日期") {
			return true
		}
	}
	return false
}

func hasHeaderAhead(lines []string, from int, names ...string) bool {
	last := from + quoteLookahead
	if last >= len(lines) {
		last = len(lines) - 1
	}
	for i := from + 1; i <= last; i++ {
		if isHeader(lines[i], names...) {
			return true
		}
	}
	return false
}

func isHeader(line string, names ...string) bool {
	name, ok := headerName(line)
	if !ok {
		return false
	}
	for _, want := range names {
		if name == want {
			return true
		}
	}
	return false
}

// headerName 取出一行信头冒号前的名字。以 > 开头时返回 false，避免把聊天引用当成邮件头。
func headerName(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ">") {
		return "", false
	}
	idx := strings.IndexAny(line, ":：")
	if idx <= 0 {
		return "", false
	}
	name := strings.ToLower(strings.TrimSpace(line[:idx]))
	if name == "" || strings.ContainsAny(name, ":：") {
		return "", false
	}
	return name, true
}

func isMailSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ">") {
		return false
	}
	core := strings.Trim(line, "-_=* \t")
	core = strings.Trim(core, ":：")
	core = strings.TrimSpace(core)
	return strings.EqualFold(core, "original message") || core == "原始邮件"
}

func isWroteLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ">") {
		return false
	}
	return onWrotePattern.MatchString(line) || zaiWrotePattern.MatchString(line)
}
