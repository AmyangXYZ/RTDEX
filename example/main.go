package main

import (
	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/engine"
)

func main() {
	engine := engine.NewEngine(config.DefaultConfig)
	engine.Start()
}
