// for easier import

package rtdex

import (
	"github.com/AmyangXYZ/rtdex/pkg/client"
	"github.com/AmyangXYZ/rtdex/pkg/config"
	"github.com/AmyangXYZ/rtdex/pkg/core"
	"github.com/AmyangXYZ/rtdex/pkg/engine"
)

type Engine = core.Engine
type Client = client.Client
type Config = config.Config
type CacheItem = core.CacheItem
type PacketMeta = core.PacketMeta

var NewEngine = engine.NewEngine
var NewClient = client.NewClient
var DefaultConfig = config.DefaultConfig
