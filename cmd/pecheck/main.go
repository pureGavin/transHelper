package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: pecheck <exe>")
		os.Exit(2)
	}
	path := os.Args[1]
	found, err := checkFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "检查 %s 失败: %v\n", path, err)
		os.Exit(1)
	}
	if len(found) > 0 {
		fmt.Fprintf(os.Stderr, "%s 不符合发布要求:\n", path)
		for _, item := range found {
			fmt.Fprintf(os.Stderr, "- %s\n", item)
		}
		os.Exit(1)
	}
	fmt.Printf("%s 已启用 ASLR、高熵 ASLR 和 DEP，没有符号表\n", path)
}
