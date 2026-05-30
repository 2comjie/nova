package main

import (
	"fmt"

	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/config/file"
)

type AppConfig struct {
	Server Server `json:"server"`
}

type Server struct {
	Port       int    `json:"port"`
	InstanceId string `json:"instance-id"`
}

func main() {
	configCenter := config.New(config.WithSource(file.NewSource("examples/config/conf/server.yml")))
	err := configCenter.Load()
	if err != nil {
		panic(err)
	}

	appConfig := &AppConfig{}
	err = configCenter.Scan(&appConfig)
	if err != nil {
		panic(err)
	}

	fmt.Printf("server port: %d, instanceId: %s\n", appConfig.Server.Port, appConfig.Server.InstanceId)
}
