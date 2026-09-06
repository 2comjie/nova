package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/2comjie/nova/internal/diffgen"
)

func main() {
	dir := flag.String("dir", ".", "Go 模型目录")
	protoDir := flag.String("proto-dir", "./proto", "Proto 输出根目录")
	flag.Parse()

	if err := diffgen.Generate(*dir, *protoDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
