package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// CheckSample 是设置窗口用来试译的固定句子。
	CheckSample = "你好，这是一条测试消息。"
	// 结果写在按钮旁边，过长就截断，避免把窗口撑乱。
	checkMaxRunes = 80
)

// Check 用 client 试译 CheckSample，返回给用户看的一行结果。
// 失败时 err 仍返回，方便调用方记类别；message 不含响应正文。
func Check(ctx context.Context, client *Client) (message string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		err = fmt.Errorf("没有翻译客户端")
		return DescribeCheck("", 0, err), err
	}
	start := time.Now()
	text, err := client.Translate(ctx, CheckSample)
	return DescribeCheck(text, time.Since(start), err), err
}

// DescribeCheck 把一次试译整理成一行。成功时带上耗时和译文，失败时只说原因。
func DescribeCheck(text string, elapsed time.Duration, err error) string {
	if err != nil {
		return checkFailure(err)
	}
	text = oneLine(text, checkMaxRunes)
	if text == "" {
		return "模型返回空结果"
	}
	return "可用，耗时 " + formatCheckElapsed(elapsed) + "；译文：" + text
}

// CheckKind 返回日志用的失败类别，不含响应内容。成功时返回空字符串。
func CheckKind(err error) string {
	if err == nil {
		return ""
	}
	var te *Error
	if !errors.As(err, &te) {
		return "失败"
	}
	switch te.Kind {
	case KindTimeout:
		return "超时"
	case KindCanceled:
		return "已取消"
	case KindHTTP:
		if te.Status == 0 {
			return "连不上"
		}
		return fmt.Sprintf("HTTP %d", te.Status)
	case KindDecode:
		return "无法解析"
	case KindEmpty:
		return "空结果"
	case KindTruncated:
		return "被截断"
	default:
		return "失败"
	}
}

func checkFailure(err error) string {
	var te *Error
	if !errors.As(err, &te) {
		return "翻译失败"
	}
	switch te.Kind {
	case KindTimeout:
		return "测试超时，请加大超时或检查网络"
	case KindCanceled:
		return "测试已取消"
	case KindHTTP:
		if te.Status == 0 {
			return "连不上模型，请检查地址、网络和代理"
		}
		switch te.Status {
		case 401, 403:
			return fmt.Sprintf("模型返回 HTTP %d，请检查 API Key", te.Status)
		case 404:
			return "模型返回 HTTP 404，请检查地址和模型名称"
		default:
			return fmt.Sprintf("模型返回 HTTP %d", te.Status)
		}
	case KindDecode:
		return "模型返回无法解析，请确认是 OpenAI 兼容接口"
	case KindEmpty:
		return "模型返回空结果"
	case KindTruncated:
		return "译文过长被截断"
	default:
		return "翻译失败"
	}
}

func formatCheckElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < 100*time.Millisecond {
		return fmt.Sprintf("%d 毫秒", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f 秒", d.Seconds())
}

func oneLine(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	end := 0
	n := 0
	for end < len(s) && n < maxRunes {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
		n++
	}
	return s[:end] + "…"
}
