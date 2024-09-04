package main

import (
	"fmt"
	"time"

	"github.com/AmyangXYZ/rtdex"
)

func main() {
	engine := rtdex.NewEngine(rtdex.DefaultConfig)
	go engine.Start()
	time.Sleep(2 * time.Second)
	client := rtdex.NewClient(3, "namespace-A", rtdex.DefaultConfig)
	client.Connect()
	client.Put("/data/test", []byte("test"), 10)

	if data, err := client.Get("/data/test"); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("data:", string(data))
	}

	client.Disconnect()
	// go func() {
	// 	for pkt := range engine.PacketSniffer().Stream() {
	// 		fmt.Println(pkt)
	// 	}
	// }()
	go engine.Stop()
	select {}
}
