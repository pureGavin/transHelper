package main

import (
	"debug/pe"
	"fmt"
	"strings"
)

// problems 检查发布用 exe 的保护标志和符号表。
// chars 是可选头里的 DllCharacteristics，symbols 是 COFF 符号数量。
func problems(chars uint16, symbols uint32, sections []string) []string {
	var out []string
	if chars&pe.IMAGE_DLLCHARACTERISTICS_DYNAMIC_BASE == 0 {
		out = append(out, "没有 ASLR（DYNAMIC_BASE）")
	}
	if chars&pe.IMAGE_DLLCHARACTERISTICS_HIGH_ENTROPY_VA == 0 {
		out = append(out, "没有 64 位高熵 ASLR（HIGH_ENTROPY_VA）")
	}
	if chars&pe.IMAGE_DLLCHARACTERISTICS_NX_COMPAT == 0 {
		out = append(out, "没有 DEP（NX_COMPAT）")
	}
	if symbols != 0 {
		out = append(out, fmt.Sprintf("符号表不为空（%d 个）", symbols))
	}
	for _, name := range sections {
		// 段名最长 8 字节，不足时用 NUL 填充。
		trimmed := strings.TrimRight(name, "\x00")
		if strings.HasPrefix(trimmed, ".debug") {
			out = append(out, "有调试段 "+trimmed)
		}
	}
	return out
}

func checkFile(path string) ([]string, error) {
	file, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var chars uint16
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		chars = header.DllCharacteristics
	default:
		return []string{"不是 64 位 PE"}, nil
	}
	names := make([]string, 0, len(file.Sections))
	for _, section := range file.Sections {
		names = append(names, section.Name)
	}
	return problems(chars, file.FileHeader.NumberOfSymbols, names), nil
}
