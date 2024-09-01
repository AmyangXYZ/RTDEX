package main

import (
	"fmt"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/client"
	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/engine"
)

func main() {
	engine := engine.NewEngine(config.DefaultConfig)
	go engine.Start()
	time.Sleep(time.Millisecond * 500)

	client := client.NewClient(3, "t", config.DefaultConfig)
	client.Connect()
	client.Put("/data/test", []byte("test"), 1)

	data, err := client.Get("/data/test", time.Second*10)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("data:", string(data))

	client.Disconnect()
	go engine.Stop()
	select {}
}
