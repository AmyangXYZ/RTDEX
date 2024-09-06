package cache

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/AmyangXYZ/rtdex/internal/utils"
	"github.com/AmyangXYZ/rtdex/pkg/core"
)

type Node struct {
	name     string
	segment  string
	value    *core.CacheItem
	parent   *Node
	children sync.Map
}

type Cache struct {
	engine      core.Engine
	root        *Node
	expiryQueue *utils.ExpiryPriorityQueue
	logger      *log.Logger
}

// NewCache creates and initializes a new Cache instance.
// It starts a goroutine for housekeeping and returns a pointer to the new Cache.
func NewCache(engine core.Engine) *Cache {
	cache := &Cache{
		engine:      engine,
		root:        &Node{segment: ""},
		expiryQueue: utils.NewExpiryPriorityQueue(),
		logger:      log.New(log.Writer(), "[Cache] ", 0),
	}
	go cache.Housekeeping()
	return cache
}

func (c *Cache) Set(name string, item *core.CacheItem) {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		child, _ := node.children.LoadOrStore(segment, &Node{
			name:    name,
			segment: segment,
			parent:  node,
		})
		node = child.(*Node)
	}

	node.value = item
	c.expiryQueue.UpdateExpiration(node, item.Expiry)
}

func (c *Cache) Get(name string) *core.CacheItem {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		if child, ok := node.children.Load(segment); !ok {
			return nil
		} else {
			node = child.(*Node)
		}
	}
	if node.value != nil && time.Now().Before(node.value.Expiry) {
		return node.value
	}

	return nil
}

func (c *Cache) GetAll() []*core.CacheItem {
	result := make([]*core.CacheItem, 0)
	queue := []*Node{c.root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.value != nil {
			result = append(result, node.value)
		}
		node.children.Range(func(_, value interface{}) bool {
			child := value.(*Node)
			queue = append(queue, child)
			return true
		})
	}
	return result
}

func (c *Cache) Housekeeping() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-c.engine.Ctx().Done():
			c.ClearAll()
			return
		case <-ticker.C:
			now := time.Now()
			for {
				item := c.expiryQueue.Peek()
				if item == nil || item.ExpireAt.After(now) {
					break
				}
				c.expiryQueue.Pop()
				node := item.Value.(*Node)
				c.logger.Printf("Removed expired cache %s\n", item.Value.(*Node).value.Name)
				node.value = nil
				node.parent.children.Delete(node.segment)
			}
		}
	}
}

func (c *Cache) ClearAll() {
	c.root = &Node{
		children: sync.Map{},
	}
	c.expiryQueue = utils.NewExpiryPriorityQueue()
	c.logger.Println("All cache cleared")
}
