package main

import (
	"crypto/rand"
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

	data := make([]byte, 1024*1024*2)
	rand.Read(data)

	client.Put("/data/test", data, 100)

	if data, err := client.Get("/data/test"); err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("Received data: %d bytes\n", len(data))
	}

	client.Disconnect()
	// // go func() {
	// // 	for pkt := range engine.PacketSniffer().Stream() {
	// // 		fmt.Println(pkt)
	// // 	}
	// // }()
	// go engine.Stop()
	select {}
}
