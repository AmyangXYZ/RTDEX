package main

import (
	"fmt"
	"time"

	"github.com/AmyangXYZ/rtdex"
)

func main() {
	engine := rtdex.NewEngine(rtdex.DefaultConfig)
	go engine.Start()
	time.Sleep(1 * time.Second)
	client := rtdex.NewClient(3, "namespace-A", rtdex.DefaultConfig)
	client.Connect()
	client.Put("/data/test", []byte("test"), 10)

	if data, err := client.Get("/data/test"); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("data:", string(data))
	}

	client.Disconnect()
	// packets := engine.PacketSniffer().Get(0, 100)
	// for _, pkt := range packets {
	// 	fmt.Println(pkt.Payload)
	// }
	go engine.Stop()
	select {}
}
