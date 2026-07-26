package main

import (
	"github.com/2comjie/wali/app/gate"
	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/config/file"
	"github.com/2comjie/wali/logx"
)

type TestConfig struct {
	Name   string       `json:"name"`
	Age    int          `json:"age"`
	Server ServerConfig `json:"server"`
}

type ServerConfig struct {
	RPCHost string `json:"rpc_host"`
	RPCPort int    `json:"rpc_port"`
}

func main() {
	fileConfig := file.NewSource("./config_center/config.yml")
	center := config.New(config.WithSource(fileConfig))
	defer center.Close()

	if err := center.Load(); err != nil {
		panic(err)
	}

	var testConfig TestConfig
	if err := center.Scan(&testConfig); err != nil {
		panic(err)
	}

	routes, err := config.Get[[]gate.Route](center, "gate.routes")
	if err != nil {
		panic(err)
	}

	logx.Infof(
		"name=%s age=%d rpc=%s:%d gate_service=%s",
		testConfig.Name,
		testConfig.Age,
		testConfig.Server.RPCHost,
		testConfig.Server.RPCPort,
		routes[0].Target.Service,
	)
}
