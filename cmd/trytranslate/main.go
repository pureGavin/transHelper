// Command trytranslate 用配置里的模型翻译 testdata/samples.txt，方便人工检查译文。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"transhelper/internal/config"
	"transhelper/internal/translate"
)

func main() {
	configPath := flag.String("config", "transhelper.toml", "配置文件路径")
	samplesPath := flag.String("samples", filepath.Join("testdata", "samples.txt"), "样例句文件，用单独一行的 --- 分隔")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := os.ReadFile(*samplesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取样例句: %v\n", err)
		os.Exit(1)
	}
	samples := splitSamples(string(data))
	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "样例句是空的")
		os.Exit(1)
	}

	client := &translate.Client{
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Prompt:      cfg.SystemPrompt,
	}
	failed := 0
	for i, sample := range samples {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		start := time.Now()
		out, err := client.Translate(ctx, sample)
		elapsed := time.Since(start).Round(time.Millisecond)
		cancel()

		fmt.Printf("----- %d (%s) -----\n%s\n=> ", i+1, elapsed, sample)
		if err != nil {
			failed++
			fmt.Printf("错误: %v\n\n", err)
			continue
		}
		fmt.Printf("%s\n\n", out)
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func splitSamples(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n---\n")
	samples := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			samples = append(samples, part)
		}
	}
	return samples
}
