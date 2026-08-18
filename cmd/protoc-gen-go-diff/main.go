package main

import (
	"flag"

	"github.com/2comjie/nova/internal/diffgen"
	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	flags := flag.NewFlagSet("protoc-gen-go-diff", flag.ContinueOnError)
	protogen.Options{ParamFunc: flags.Set}.Run(diffgen.Generate)
}
