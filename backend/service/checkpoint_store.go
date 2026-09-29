package service

import (
	"context"
	"sync"
	"time"
)

// checkpointTTL checkpoint 在内存中的存活时间: 超过未访问即淘汰,
// 防止每个 (user, history) 的 checkpoint 永久累积导致内存持续增长
const checkpointTTL = 24 * time.Hour

type checkpointEntry struct {
	data       []byte
	lastAccess time.Time
}

type InMemoryCheckPointStore struct {
	mu    sync.RWMutex
	store map[string]checkpointEntry
}

func NewInMemoryCheckPointStore() *InMemoryCheckPointStore {
	return &InMemoryCheckPointStore{
		store: make(map[string]checkpointEntry),
	}
}

func (s *InMemoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.store[checkPointID]
	if !ok {
		return nil, false, nil
	}
	// 已过期的视为不存在并清理 (工具审批的挂起 checkpoint 超过 TTL 即失效)
	if time.Since(entry.lastAccess) > checkpointTTL {
		delete(s.store, checkPointID)
		return nil, false, nil
	}
	entry.lastAccess = time.Now()
	s.store[checkPointID] = entry
	return entry.data, true, nil
}

func (s *InMemoryCheckPointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 惰性清理: 写入时顺带淘汰过期条目 (条目量 = 活跃会话数, 遍历成本可忽略)
	now := time.Now()
	for id, entry := range s.store {
		if now.Sub(entry.lastAccess) > checkpointTTL {
			delete(s.store, id)
		}
	}
	s.store[checkPointID] = checkpointEntry{data: checkPoint, lastAccess: now}
	return nil
}
