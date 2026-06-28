package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/external"
	pb "github.com/2comjie/wali/examples/rpcdemo/pbDemo"
	redisLoc "github.com/2comjie/wali/locator/redis"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"github.com/2comjie/wali/rpc/client"
	"github.com/2comjie/wali/rpc/lx"
)

func main() {
	rdb := external.RedisClient()
	discover := redisRegistry.NewDiscover(rdb)
	loc := redisLoc.NewProvider(rdb)

	conn, err := client.Dial(discover, loc)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	c := pb.NewHayServiceClient(conn)

	fmt.Println("demo client started. 输入格式:")
	fmt.Println("  balance <service> <name>   - 加权轮询")
	fmt.Println("  direct  <addr> <name>     - 直连")
	fmt.Println("  select  <name_key> <name> - 按 key 路由")
	fmt.Println("  node    <node_id> <name>  - 指定节点")
	fmt.Println("  quit                     - 退出")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for fmt.Print("> "); scanner.Scan(); fmt.Print("> ") {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "quit":
			fmt.Println("bye")
			return

		case "balance":
			if len(parts) < 3 {
				fmt.Println("用法: balance <service> <name>")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ctx = lx.WithBalance(ctx, parts[1])
			resp, err := c.SayHay(ctx, &pb.HayRequest{Name: parts[2]})
			cancel()
			printResult("balance", resp, err)

		case "direct":
			if len(parts) < 3 {
				fmt.Println("用法: direct <addr> <name>")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ctx = lx.WithDirect(ctx, parts[1])
			resp, err := c.SayHay(ctx, &pb.HayRequest{Name: parts[2]})
			cancel()
			printResult("direct", resp, err)

		case "node":
			if len(parts) < 3 {
				fmt.Println("用法: select <node_id> <name>")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ctx = lx.WithNode(ctx, parts[1], parts[2])
			resp, err := c.SayHay(ctx, &pb.HayRequest{Name: parts[2]})
			cancel()
			printResult("node", resp, err)

		case "select":
			if len(parts) < 3 {
				fmt.Println("用法: select <name> <key>")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ctx = lx.WithSelect(ctx, parts[1], parts[2])
			resp, err := c.SayHay(ctx, &pb.HayRequest{Name: parts[2]})
			cancel()
			printResult("select", resp, err)

		default:
			fmt.Println("未知命令:", parts[0])
		}
	}

	util.WaitUntilSignaled()
}

func printResult(strategy string, resp *pb.HayResponse, err error) {
	if err != nil {
		fmt.Printf("[%s] error: %v\n", strategy, err)
	} else {
		fmt.Printf("[%s] %s\n", strategy, resp.Message)
	}
}
