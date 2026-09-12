package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/2comjie/nova/internal/diffgen"
)

func main() {
	config := flag.String("config", "diffgen.yaml", "生成配置文件")
	flag.Parse()

	if err := diffgen.Generate(*config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
