package cache

import (
	"log"
	"strings"
	"sync"
	"time"
)

type CacheItem struct {
	name     string
	segment  string
	value    interface{}
	expiry   time.Time
	children sync.Map
}

type Cache struct {
	root   *CacheItem
	logger *log.Logger
}

// NewCache creates and initializes a new Cache instance.
// It starts a goroutine for housekeeping and returns a pointer to the new Cache.
func NewCache() *Cache {
	cache := &Cache{
		root:   &CacheItem{segment: ""},
		logger: log.New(log.Writer(), "[Cache] ", 0),
	}
	go cache.Housekeeping()
	return cache
}

func (c *Cache) Set(name string, data interface{}, freshness time.Duration) {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		child, _ := node.children.LoadOrStore(segment, &CacheItem{
			name:    name,
			segment: segment,
		})
		node = child.(*CacheItem)
	}

	node.value = data
	node.expiry = time.Now().Add(freshness)
}

func (c *Cache) Get(name string) interface{} {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		if child, ok := node.children.Load(segment); !ok {
			return nil
		} else {
			node = child.(*CacheItem)
		}
	}
	if node.value != nil && time.Now().Before(node.expiry) {
		return node.value
	}

	return nil
}

func (c *Cache) GetAll() []interface{} {
	result := make([]interface{}, 0)
	queue := []*CacheItem{c.root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.value != nil {
			result = append(result, node.value)
		}
		node.children.Range(func(_, value interface{}) bool {
			child := value.(*CacheItem)
			queue = append(queue, child)
			return true
		})
	}
	return result
}

func (c *Cache) Housekeeping() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		queue := []*CacheItem{c.root}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			node.children.Range(func(_, value interface{}) bool {
				child := value.(*CacheItem)
				queue = append(queue, child)
				return true
			})
			if node.value != nil && time.Now().After(node.expiry) {
				c.logger.Printf("Removed expired cache %s\n", node.name)
				node.value = nil
			}
		}
	}
}
