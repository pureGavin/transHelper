package translate

import (
	"regexp"
	"strings"
)

// thinkPattern 去掉模型偶尔夹带的思考过程。(?s) 让 . 匹配换行。
var thinkPattern = regexp.MustCompile(`(?is)<think>.*?</think>`)

// quotePairs 是模型可能用来包裹整段译文的引号。
var quotePairs = [][2]string{
	{`"`, `"`},
	{"“", "”"},
	{"「", "」"},
	{"『", "』"},
}

// labels 是模型可能加在译文前面的说明。只去掉一层。
var labels = []string{"Translation:", "译文：", "译文:"}

// Clean 去掉思考过程、首尾空白，以及原文里没有的整段引号。
func Clean(original, translated string) string {
	translated = thinkPattern.ReplaceAllString(translated, "")
	translated = strings.TrimSpace(translated)
	translated = trimLabel(translated)
	translated = unwrapQuotes(original, translated)
	return strings.TrimSpace(translated)
}

func trimLabel(translated string) string {
	for _, label := range labels {
		if len(translated) < len(label) {
			continue
		}
		if !strings.EqualFold(translated[:len(label)], label) {
			continue
		}
		rest := strings.TrimSpace(translated[len(label):])
		if rest == "" {
			return translated
		}
		return rest
	}
	return translated
}

func unwrapQuotes(original, translated string) string {
	for _, pair := range quotePairs {
		open, close := pair[0], pair[1]
		if !strings.HasPrefix(translated, open) || !strings.HasSuffix(translated, close) {
			continue
		}
		if len(translated) < len(open)+len(close) {
			continue
		}
		// 原文自己带了这对引号时保留，避免把用户要发出的引号删掉。
		if strings.Contains(original, open) || strings.Contains(original, close) {
			continue
		}
		inner := strings.TrimSpace(translated[len(open) : len(translated)-len(close)])
		if inner == "" {
			continue
		}
		return inner
	}
	return translated
}
