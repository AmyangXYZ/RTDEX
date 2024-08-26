package main

import (
	"testing"
	"time"
)

func TestSetAndGet(t *testing.T) {
	cache := NewCache()
	data := &CachedData{
		Name:   "/test/data",
		Size:   100,
		Expiry: time.Now().Add(1 * time.Hour),
	}
	cache.Set("/test/data", data)

	retrieved := cache.Get("/test/data")
	if retrieved == nil {
		t.Errorf("Expected to retrieve data, got nil")
		return
	}
	if retrieved.Name != data.Name {
		t.Errorf("Expected name %s, got %s", data.Name, retrieved.Name)
	}
}

func TestGetNonExistent(t *testing.T) {
	cache := NewCache()
	retrieved := cache.Get("/non/existent")
	if retrieved != nil {
		t.Errorf("Expected nil for non-existent key, got %v", retrieved)
	}
}

func TestGetAll(t *testing.T) {
	cache := NewCache()
	cache.Set("/data1", &CachedData{Name: "/data1", Size: 100, Expiry: time.Now().Add(1 * time.Hour)})
	cache.Set("/data2", &CachedData{Name: "/data2", Size: 200, Expiry: time.Now().Add(1 * time.Hour)})
	cache.Set("/data3", &CachedData{Name: "/data3", Size: 300, Expiry: time.Now().Add(1 * time.Hour)})
	cache.Set("/dataset1/data3", &CachedData{Name: "/data3", Size: 300, Expiry: time.Now().Add(1 * time.Hour)})

	results := cache.GetAll()
	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}
}

func TestExpiration(t *testing.T) {
	cache := NewCache()
	cache.Set("/expired", &CachedData{Name: "/expired", Size: 100, Expiry: time.Now().Add(-1 * time.Hour)})
	retrieved := cache.Get("/expired")
	if retrieved != nil {
		t.Errorf("Expected nil for expired data, got %v", retrieved)
	}
}
