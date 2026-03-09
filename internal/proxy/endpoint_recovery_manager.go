package proxy

import (
	"fmt"
	"log/slog"
	"sync"
)

// EndpointRecoverySignalManager 管理端点恢复信号
// 当端点请求成功时，通知所有等待该端点的挂起请求
type EndpointRecoverySignalManager struct {
	// 端点成功信号订阅者 map[endpointName][]chan string
	subscribers map[string][]chan string
	mutex       sync.RWMutex
}

// NewEndpointRecoverySignalManager 创建端点恢复信号管理器
func NewEndpointRecoverySignalManager() *EndpointRecoverySignalManager {
	return &EndpointRecoverySignalManager{
		subscribers: make(map[string][]chan string),
	}
}

// BroadcastEndpointSuccess 广播端点成功信号
// 当某个端点请求成功时调用，通知所有等待该端点的挂起请求
func (ersm *EndpointRecoverySignalManager) BroadcastEndpointSuccess(endpointName string) {
	ersm.mutex.Lock()
	defer ersm.mutex.Unlock()

	if subscribers, exists := ersm.subscribers[endpointName]; exists && len(subscribers) > 0 {
		slog.Info(fmt.Sprintf("📡 [端点恢复广播] 端点 %s 成功，通知 %d 个等待请求",
			endpointName, len(subscribers)))

		// 通知所有订阅者
		for i, ch := range subscribers {
			select {
			case ch <- endpointName:
				slog.Debug(fmt.Sprintf("✅ [恢复通知] 端点 %s 已通知等待者 %d", endpointName, i+1))
			default:
				slog.Warn(fmt.Sprintf("⚠️ [通知失败] 端点 %s 等待者 %d 通道已满或已关闭", endpointName, i+1))
			}
		}

		// 清空该端点的订阅者列表
		delete(ersm.subscribers, endpointName)
		slog.Debug(fmt.Sprintf("🧹 [清理订阅] 端点 %s 订阅者列表已清空", endpointName))
	}
}

// Subscribe 订阅端点恢复信号
// 返回一个channel，当端点恢复时会收到端点名称
func (ersm *EndpointRecoverySignalManager) Subscribe(endpointName string) chan string {
	ersm.mutex.Lock()
	defer ersm.mutex.Unlock()

	// 创建带缓冲的channel，避免阻塞
	ch := make(chan string, 1)

	// 添加到订阅者列表
	ersm.subscribers[endpointName] = append(ersm.subscribers[endpointName], ch)

	subscriberCount := len(ersm.subscribers[endpointName])
	slog.Debug(fmt.Sprintf("🔔 [端点订阅] 新订阅者监听端点 %s，当前总订阅者: %d",
		endpointName, subscriberCount))

	return ch
}

// Unsubscribe 取消订阅端点恢复信号
// 从订阅者列表中移除指定的channel
func (ersm *EndpointRecoverySignalManager) Unsubscribe(endpointName string, ch chan string) {
	ersm.mutex.Lock()
	defer ersm.mutex.Unlock()

	if subscribers, exists := ersm.subscribers[endpointName]; exists {
		// 查找并移除指定的channel
		for i, subscriber := range subscribers {
			if subscriber == ch {
				// 移除该订阅者
				ersm.subscribers[endpointName] = append(subscribers[:i], subscribers[i+1:]...)
				slog.Debug(fmt.Sprintf("🔕 [取消订阅] 端点 %s 移除订阅者，剩余订阅者: %d",
					endpointName, len(ersm.subscribers[endpointName])))
				break
			}
		}

		// 如果该端点没有订阅者了，删除映射
		if len(ersm.subscribers[endpointName]) == 0 {
			delete(ersm.subscribers, endpointName)
			slog.Debug(fmt.Sprintf("🧹 [清理映射] 端点 %s 无订阅者，已清理映射", endpointName))
		}
	}

	// 关闭channel
	close(ch)
}

// GetSubscriberCount 获取指定端点的订阅者数量（用于测试和监控）
func (ersm *EndpointRecoverySignalManager) GetSubscriberCount(endpointName string) int {
	ersm.mutex.RLock()
	defer ersm.mutex.RUnlock()

	if subscribers, exists := ersm.subscribers[endpointName]; exists {
		return len(subscribers)
	}
	return 0
}

// GetTotalSubscriberCount 获取所有端点的订阅者总数（用于监控）
func (ersm *EndpointRecoverySignalManager) GetTotalSubscriberCount() int {
	ersm.mutex.RLock()
	defer ersm.mutex.RUnlock()

	total := 0
	for _, subscribers := range ersm.subscribers {
		total += len(subscribers)
	}
	return total
}
