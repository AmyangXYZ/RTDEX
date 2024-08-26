package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CachedData struct {
	Name      string    `json:"name"`
	Owner     uint32    `json:"owner"`
	Size      int       `json:"size"`
	Expiry    time.Time `json:"expiry"`
	Downloads int       `json:"downloads"`
	Checksum  uint32    `json:"checksum"`
	data      []byte
}

type TrieNode struct {
	segment  string
	value    *CachedData
	parent   *TrieNode
	children sync.Map
	hasChunk bool
}

type Cache struct {
	root *TrieNode
}

func NewCache() *Cache {
	cache := &Cache{root: &TrieNode{segment: ""}}
	go cache.housekeeping()
	return cache
}

func (c *Cache) Set(name string, data *CachedData) {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		child, _ := node.children.LoadOrStore(segment, &TrieNode{
			parent:  node,
			segment: segment,
		})
		node = child.(*TrieNode)
	}
	if _, err := strconv.Atoi(node.segment); err == nil {
		node.parent.hasChunk = true
	}
	node.value = data
}

func (c *Cache) Get(name string) *CachedData {
	segments := strings.Split(strings.Trim(name, "/"), "/")
	node := c.root
	for _, segment := range segments {
		if child, ok := node.children.Load(segment); !ok {
			return nil
		} else {
			node = child.(*TrieNode)
		}
	}
	if node.value != nil && time.Now().Before(node.value.Expiry) {
		return node.value
	}

	return nil
}

func (c *Cache) GetAll() []*CachedData {
	result := make([]*CachedData, 0)
	queue := []*TrieNode{c.root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.value != nil {
			result = append(result, node.value)
		}
		node.children.Range(func(_, value interface{}) bool {
			child := value.(*TrieNode)
			queue = append(queue, child)
			return true
		})
	}
	return result
}

func (c *Cache) housekeeping() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		queue := []*TrieNode{c.root}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			node.children.Range(func(_, value interface{}) bool {
				child := value.(*TrieNode)
				queue = append(queue, child)
				return true
			})
			if node.value != nil && time.Now().After(node.value.Expiry) {
				fmt.Printf("[Cache] Removed expired cache %s\n", node.value.Name)
				node.value = nil
			}
		}
	}
}
