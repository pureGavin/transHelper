package app

import (
	"strings"
	"testing"
)

func TestHasMailQuote(t *testing.T) {
	positives := []string{
		"From: Alice\nSent: Monday\n你好",
		"发件人：张三\n发送时间：2026年11月17日\n收到",
		"发件人: 张三\n\n日期: 周一\n正文",
		"-----Original Message-----",
		"-------- 原始邮件 --------",
		"On Tue, Nov 17, 2026 at 4:00 PM Bob wrote:",
		"在 2026年11月17日，张三 写道：",
		"于 2026年11月17日周一 16:14，张三 写道:",
	}
	for _, text := range positives {
		if !hasMailQuote(text) {
			t.Errorf("应该认出引用:\n%s", text)
		}
	}

	negatives := []string{
		"你好，明天开会",
		"> 我觉得可以\n> 你看呢",
		"> From: Alice\n> Sent: Monday",
		"From: Alice 只是聊天里的一句话",
		"日期: 明天记得带上文件",
		"请查收原始邮件附件",
		"From: Alice\n" + strings.Repeat("其它\n", quoteLookahead+1) + "Sent: Monday",
	}
	for _, text := range negatives {
		if hasMailQuote(text) {
			t.Errorf("不该当成引用:\n%s", text)
		}
	}
}
