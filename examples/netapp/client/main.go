package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/2comjie/wali/core/log"
	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/network/tcp"
	"github.com/2comjie/wali/packet"
	"go.uber.org/zap"
)

func main() {
	log.DevFile("logs/app.log")
	defer zap.L().Sync()

	tcpClient := tcp.NewClient(tcp.WithAddr(":8080"))
	err := tcpClient.Connect(
		network.WithOnMessage(func(conn network.Conn, msg packet.Message) {
			zap.S().Infof("recv msg %v %v %v", msg.MessageType(), msg.Seq(), msg.Route())
		}),
		network.WithOnDisconnect(func(conn network.Conn) {
			zap.S().Infof("disconnected")
		}),
	)
	if err != nil {
		zap.S().Errorf("connect err %v", err)
		return
	}
	defer tcpClient.Close()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("input (ctrl+d to quit):")
	for fmt.Print("> "); scanner.Scan(); fmt.Print("> ") {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "quit" || text == "exit" {
			return
		}
		rsp, err := tcpClient.Call(1, []byte(text))
		if err != nil {
			zap.S().Errorf("call err %v", err)
			continue
		}
		fmt.Printf("recv: %s\n", string(rsp.Data()))
	}
}
