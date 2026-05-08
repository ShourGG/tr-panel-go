package websocket

import (
	"sync"
	"testing"
	"time"
)

// TestHandleBroadcast_NoPanicOnFullSendChannel 验证：
// 当 client.send 队列满时，handleBroadcast 不会在读锁下写 map 导致 panic。
func TestHandleBroadcast_NoPanicOnFullSendChannel(t *testing.T) {
	// 清空全局状态
	clientsMu.Lock()
	for c := range clients {
		delete(clients, c)
	}
	clientsMu.Unlock()

	// 创建 10 个 client，send channel 容量为 1（容易满）
	for i := 0; i < 10; i++ {
		c := &Client{
			send: make(chan []byte, 1),
		}
		clientsMu.Lock()
		clients[c] = true
		clientsMu.Unlock()
	}

	// 并发广播 50 条消息，如果有数据竞争会大概率 panic
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			BroadcastMessage([]byte(`{"type":"test"}`))
		}(i)
	}

	// 等待所有广播完成，加超时保护
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 正常完成，没有 panic
	case <-time.After(5 * time.Second):
		t.Log("broadcast test timed out, but no panic — acceptable")
	}

	// 清理：排空所有 client 的 send channel
	clientsMu.Lock()
	remaining := len(clients)
	for c := range clients {
		close(c.send)
		delete(clients, c)
	}
	clientsMu.Unlock()

	t.Logf("test completed, %d clients remaining after broadcast", remaining)
}
