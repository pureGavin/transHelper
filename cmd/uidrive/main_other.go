//go:build !(windows && uidrive)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "uidrive 只支持 Windows，并且要用 -tags uidrive 编译")
	os.Exit(1)
}
