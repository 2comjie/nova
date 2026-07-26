package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

var errUsage = errors.New("wali: 命令参数错误")

const defaultWaliVersion = "v0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		printUsage(output)
		return errUsage
	}

	switch args[0] {
	case "new":
		return runNew(args[1:], output)
	case "add":
		return runAdd(args[1:], output)
	case "help", "-h", "--help":
		printUsage(output)
		return nil
	default:
		return fmt.Errorf("%w: 未知命令 %q", errUsage, args[0])
	}
}

func runNew(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: 用法 wali new <module> [--dir=<目录>]", errUsage)
	}
	moduleName := args[0]
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("dir", "", "项目目录")
	waliVersion := flags.String(
		"wali-version",
		defaultWaliVersion,
		"Wali模块版本",
	)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errUsage
	}

	root, err := newProject(moduleName, *directory, *waliVersion)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "项目创建完成: %s\n", root)
	_, _ = fmt.Fprintln(output, "下一步: 进入项目目录后执行 wali add node <name>")
	return nil
}

func runAdd(args []string, output io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("%w: 用法 wali add node|route <name>", errUsage)
	}
	switch args[0] {
	case "node":
		if len(args) != 2 {
			return fmt.Errorf("%w: 用法 wali add node <name>", errUsage)
		}
		root, err := findProject()
		if err != nil {
			return err
		}
		if err := addNode(root, args[1]); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(output, "Node创建完成: %s\n", args[1])
		return nil
	case "route":
		return runAddRoute(args[1:], output)
	default:
		return fmt.Errorf("%w: 未知add类型 %q", errUsage, args[0])
	}
}

func runAddRoute(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"%w: 用法 wali add route <name> --route=<id> --node=<node> [--call|--tell]",
			errUsage,
		)
	}
	routeName := args[0]
	flags := flag.NewFlagSet("add route", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	routeID := flags.Uint("route", 0, "Route ID")
	nodeName := flags.String("node", "", "目标Node")
	call := flags.Bool("call", false, "Call请求")
	tell := flags.Bool("tell", false, "Tell请求")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*call && *tell) {
		return errUsage
	}
	if !*call && !*tell {
		*call = true
	}
	if *routeID > uint(^uint32(0)) {
		return fmt.Errorf("wali: route超出uint32范围: %d", *routeID)
	}

	root, err := findProject()
	if err != nil {
		return err
	}
	if err := addRoute(root, RouteSpec{
		Name:  routeName,
		ID:    uint32(*routeID),
		Node:  *nodeName,
		Reply: *call,
	}); err != nil {
		return err
	}
	mode := "Call"
	if *tell {
		mode = "Tell"
	}
	_, _ = fmt.Fprintf(
		output,
		"Route创建完成: name=%s id=%d node=%s mode=%s\n",
		routeName,
		*routeID,
		*nodeName,
		mode,
	)
	return nil
}

func printUsage(output io.Writer) {
	_, _ = fmt.Fprintln(output, `Wali 游戏服务器脚手架

用法:
  wali new <module> [--dir=<目录>]
  wali add node <name>
  wali add route <name> --route=<id> --node=<node> [--call|--tell]`)
}
