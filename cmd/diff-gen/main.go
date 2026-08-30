package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/2comjie/nova/internal/diffgen"
)

func main() {
	dir := flag.String("dir", ".", "需要生成diff代码的Go包目录")
	flag.Parse()

	files, err := diffgen.Generate(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, file := range files {
		fmt.Println(file)
	}
}
