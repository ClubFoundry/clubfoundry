// Package auth provides durable bearer-token authentication for sidecar HTTP endpoints.
package auth

import "sync"

// Token is the concurrency-safe runtime bearer credential holder.
type Token struct {
	mu    sync.RWMutex
	value string
}
