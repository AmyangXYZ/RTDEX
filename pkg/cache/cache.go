package cache

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/AmyangXYZ/rtdex/pkg/core"
)

type Node struct {
	name     string
	segment  string
	value    *core.CacheItem
	children sync.Map
}

type Cache struct {
	root   *Node
	logger *log.Logger
}

// NewCache creates and initializes a new Cache instance.
// It starts a goroutine for housekeeping and returns a pointer to the new Cache.
func NewCache() *Cache {
	cache := &Cache{
		root:   &Node{segment: ""},
		logger: log.New(log.Writer(), "[Cache] ", 0),
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
		})
		node = child.(*Node)
	}

	node.value = item
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
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		queue := []*Node{c.root}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			node.children.Range(func(_, value interface{}) bool {
				child := value.(*Node)
				queue = append(queue, child)
				return true
			})
			if node.value != nil && time.Now().After(node.value.Expiry) {
				c.logger.Printf("Removed expired cache %s\n", node.name)
				node.value = nil
			}
		}
	}
}
