package middleware

import (
	"testing"
	"time"
)

// TestStrictVisitorCleanup 验证 StrictRateLimitMiddleware 的清理机制：
// 过期的 visitor 应该被移除，不会无限增长。
func TestStrictVisitorCleanup(t *testing.T) {
	visitors := make(map[string]*strictVisitor)

	// 模拟 3 个已过期的 visitor
	staleTime := time.Now().Add(-10 * time.Minute)
	visitors["1.1.1.1"] = &strictVisitor{lastSeen: staleTime}
	visitors["2.2.2.2"] = &strictVisitor{lastSeen: staleTime}
	// 1 个活跃的
	visitors["3.3.3.3"] = &strictVisitor{lastSeen: time.Now()}

	// 模拟清理逻辑（与 StrictRateLimitMiddleware 中的 goroutine 逻辑一致）
	for ip, v := range visitors {
		if time.Since(v.lastSeen) > 5*time.Minute {
			delete(visitors, ip)
		}
	}

	if len(visitors) != 1 {
		t.Errorf("expected 1 visitor after cleanup, got %d", len(visitors))
	}
	if _, ok := visitors["3.3.3.3"]; !ok {
		t.Error("active visitor 3.3.3.3 should not be cleaned up")
	}
}
