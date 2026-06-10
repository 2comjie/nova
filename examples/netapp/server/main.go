package main

import (
	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/log"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/network/tcp"
	"github.com/2comjie/wali/packet"
	"go.uber.org/zap"
)

func main() {
	log.DevFile("logs/app.log")
	defer zap.L().Sync()

	packer := packet.NewPacker()
	tcpServer := tcp.New(tcp.WithAddr(":8080"))
	err := tcpServer.Start(
		network.WithOnHeartbeat(func(conn network.Conn) {
			zap.S().Infof("heartbeat")
		}),
		network.WithPacker(packer),
		network.WithOnMessage(func(conn network.Conn, message packet.Message) {
			zap.S().Infof("recv msg %v %v %v", message.MessageType(), message.Seq(), message.Route())
			zap.S().Infof("msg %s", string(message.Data())) // 实际的数据
			if string(message.Data()) == "over" {
				conn.Close("over")
				return
			}

			writer := buffer.MallocWriter(len(message.Data()))
			_, writeErr := writer.Write(message.Data())
			if writeErr != nil {
				zap.S().Errorf("write err %v", writeErr)
			} else {
				rspBuff, packErr := packer.PackBuffer(packet.Rsp, message.Route(), message.Seq(), writer)
				if packErr != nil {
					zap.S().Errorf("pack err %v", packErr)
					return
				}
				pushErr := conn.Push(rspBuff)
				if pushErr != nil {
					zap.S().Errorf("push err %v", pushErr)
				}
			}
		}),
		network.WithOnStart(func() {
			zap.S().Infof("server start")
		}),
		network.WithBeforeStop(func() {
			zap.S().Infof("before server stop")
		}),
		network.WithOnStop(func() {
			zap.S().Infof("server stop")
		}),
		network.WithOnConnect(func(conn network.Conn) {
			zap.S().Infof("connect")
		}),
		network.WithOnDisconnect(func(conn network.Conn) {
			zap.S().Infof("conn on disconnect")
		}),
	)
	if err != nil {
		zap.S().Errorf("server start err %v", err)
		return
	}

	util.WaitUntilSignaled()
	err = tcpServer.Stop()
	if err != nil {
		zap.S().Errorf("server stop err %v", err)
	}
}
