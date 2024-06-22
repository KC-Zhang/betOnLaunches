package main

import (
    "sync"
    "time"
)

type Cache struct {
    HNStories  []Story
    PHPosts    []PHPost
    FetchedAt  time.Time
    Mutex      sync.Mutex
}

var cache = &Cache{}

const cacheDuration = time.Hour

func (c *Cache) isExpired() bool {
    return time.Since(c.FetchedAt) > cacheDuration
}

func (c *Cache) setCache(hnStories []Story, phPosts []PHPost) {
    c.Mutex.Lock()
    defer c.Mutex.Unlock()
    c.HNStories = hnStories
    c.PHPosts = phPosts
    c.FetchedAt = time.Now()
}

func (c *Cache) getCache() ([]Story, []PHPost) {
    c.Mutex.Lock()
    defer c.Mutex.Unlock()
    return c.HNStories, c.PHPosts
}
