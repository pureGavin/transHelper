package translate

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name     string
		original string
		raw      string
		want     string
	}{
		{
			name:     "思考过程",
			original: "你好",
			raw:      "<think>先分析语气</think>\n\nHello",
			want:     "Hello",
		},
		{
			name:     "跨行思考",
			original: "你好",
			raw:      "<THINK>第一行\n第二行</think>Hello there",
			want:     "Hello there",
		},
		{
			name:     "去掉英文引号",
			original: "你好",
			raw:      `"Hello"`,
			want:     "Hello",
		},
		{
			name:     "去掉中文引号",
			original: "你好",
			raw:      "“Hello”",
			want:     "Hello",
		},
		{
			name:     "原文有引号则保留",
			original: `他说"你好"`,
			raw:      `"He said "hello""`,
			want:     `"He said "hello""`,
		},
		{
			name:     "去掉标签",
			original: "你好",
			raw:      "Translation: Hello",
			want:     "Hello",
		},
		{
			name:     "保留换行",
			original: "甲\n乙",
			raw:      "A\nB",
			want:     "A\nB",
		},
		{
			name:     "空引号不当作译文",
			original: "你好",
			raw:      `""`,
			want:     `""`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Clean(tt.original, tt.raw)
			if got != tt.want {
				t.Fatalf("Clean() = %q, 期望 %q", got, tt.want)
			}
		})
	}
}
