package main

import (
	"fmt"

	"github.com/AmyangXYZ/rtdex"
)

func main() {
	engine := rtdex.NewEngine(rtdex.DefaultConfig)
	go engine.Start()

	client := rtdex.NewClient(3, "type-A-device", rtdex.DefaultConfig)
	client.Connect()
	client.Put("/data/test", []byte("test"), 10)

	if data, err := client.Get("/data/test"); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("data:", string(data))
	}

	client.Disconnect()
	go engine.Stop()
	select {}
}
