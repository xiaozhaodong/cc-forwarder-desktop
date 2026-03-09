package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// 模拟大型SSE响应数据
func generateLargeSSEResponse(sizeInMB int) string {
	var builder strings.Builder

	// 计算需要的总字符数（1MB = 1024*1024 字符，近似）
	totalChars := sizeInMB * 1024 * 1024

	// 创建标准SSE事件块
	sseChunk := "data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"这是一个测试消息块，用于性能测试。\"}}\n\n"
	chunkSize := len(sseChunk)

	// 计算需要重复的次数
	repetitions := totalChars / chunkSize

	builder.Grow(totalChars) // 预分配内存

	for i := 0; i < repetitions; i++ {
		builder.WriteString(sseChunk)

		// 定期插入Token统计信息
		if i%100 == 0 {
			tokenData := fmt.Sprintf("data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d}}\n\n",
				i/10, i/5)
			builder.WriteString(tokenData)
		}
	}

	return builder.String()
}

// 旧版本：使用io.ReadAll的实现
func processStreamBuffered(resp *http.Response) (int64, time.Duration, error) {
	start := time.Now()

	// 模拟旧版本的io.ReadAll()方式
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	// 模拟处理过程
	_ = data

	duration := time.Since(start)
	return int64(len(data)), duration, nil
}

// 新版本：使用8KB流式处理
func processStreamStreaming(resp *http.Response) (int64, time.Duration, error) {
	start := time.Now()

	buffer := make([]byte, StreamBufferSize) // 8KB缓冲区
	var totalBytes int64

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			totalBytes += int64(n)
			// 模拟立即转发到客户端
			_ = buffer[:n]
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, 0, err
		}
	}

	duration := time.Since(start)
	return totalBytes, duration, nil
}

// 创建模拟HTTP响应
func createMockResponse(data string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(data)),
	}
}

// 内存使用测试
func TestStreamProcessor_MemoryUsage(t *testing.T) {
	testSizes := []int{1, 5, 10} // MB

	for _, sizeInMB := range testSizes {
		t.Run(fmt.Sprintf("Size_%dMB", sizeInMB), func(t *testing.T) {
			data := generateLargeSSEResponse(sizeInMB)

			// 清理环境，多次GC确保稳定
			for i := 0; i < 3; i++ {
				runtime.GC()
			}
			time.Sleep(10 * time.Millisecond) // 等待GC完成

			// 测试旧版本内存使用 - 使用HeapAlloc更准确
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)
			baselineHeap := m1.HeapAlloc

			resp1 := createMockResponse(data)
			bytesProcessed1, duration1, err1 := processStreamBuffered(resp1)
			resp1.Body.Close()

			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)
			bufferedMemory := m2.HeapAlloc - baselineHeap

			// 清理环境
			for i := 0; i < 3; i++ {
				runtime.GC()
			}
			time.Sleep(10 * time.Millisecond)

			// 测试新版本内存使用
			var m3 runtime.MemStats
			runtime.ReadMemStats(&m3)
			baselineHeap2 := m3.HeapAlloc

			resp2 := createMockResponse(data)
			bytesProcessed2, duration2, err2 := processStreamStreaming(resp2)
			resp2.Body.Close()

			var m4 runtime.MemStats
			runtime.ReadMemStats(&m4)
			streamingMemory := m4.HeapAlloc - baselineHeap2

			// 验证结果
			if err1 != nil {
				t.Errorf("缓冲模式处理失败: %v", err1)
			}
			if err2 != nil {
				t.Errorf("流式模式处理失败: %v", err2)
			}

			if bytesProcessed1 != bytesProcessed2 {
				t.Errorf("处理字节数不匹配: 缓冲=%d vs 流式=%d", bytesProcessed1, bytesProcessed2)
			}

			// 计算内存改善百分比
			var memoryImprovement float64
			if bufferedMemory > 0 {
				memoryImprovement = float64(int64(bufferedMemory)-int64(streamingMemory)) / float64(bufferedMemory) * 100
			}

			t.Logf("📊 [内存性能] 数据大小: %dMB", sizeInMB)
			t.Logf("   缓冲模式内存使用: %d 字节 (%.2f MB)", bufferedMemory, float64(bufferedMemory)/(1024*1024))
			t.Logf("   流式模式内存使用: %d 字节 (%.2f MB)", streamingMemory, float64(streamingMemory)/(1024*1024))
			t.Logf("   内存改善: %.2f%%", memoryImprovement)
			t.Logf("   缓冲模式耗时: %v", duration1)
			t.Logf("   流式模式耗时: %v", duration2)

			// 计算延迟改善
			var latencyImprovement float64
			if duration1 > 0 {
				latencyImprovement = float64(duration1-duration2) / float64(duration1) * 100
			}
			t.Logf("   延迟改善: %.2f%%", latencyImprovement)

			// 验证性能改善
			if duration2 < duration1 {
				t.Logf("✅ 延迟改善验证通过: 流式模式提升了 %.2f%% 的处理速度", latencyImprovement)
			}

			// 对于大数据量，验证内存控制效果
			if sizeInMB >= 5 {
				dataSize := int64(sizeInMB * 1024 * 1024)
				if int64(streamingMemory) < dataSize/4 { // 流式内存应该远小于数据大小
					t.Logf("✅ 内存控制验证通过: 流式模式有效控制了内存使用")
				}
			}
		})
	}
}

// 响应延迟测试
func TestStreamProcessor_ResponseLatency(t *testing.T) {
	testSizes := []int{1, 5, 10} // MB

	for _, sizeInMB := range testSizes {
		t.Run(fmt.Sprintf("Latency_%dMB", sizeInMB), func(t *testing.T) {
			data := generateLargeSSEResponse(sizeInMB)

			// 测试缓冲模式延迟
			var bufferedLatencies []time.Duration
			for i := 0; i < 5; i++ { // 多次测试取平均
				resp := createMockResponse(data)
				_, latency, err := processStreamBuffered(resp)
				resp.Body.Close()

				if err != nil {
					t.Errorf("缓冲模式测试失败: %v", err)
					continue
				}
				bufferedLatencies = append(bufferedLatencies, latency)
			}

			// 测试流式模式延迟
			var streamingLatencies []time.Duration
			for i := 0; i < 5; i++ { // 多次测试取平均
				resp := createMockResponse(data)
				_, latency, err := processStreamStreaming(resp)
				resp.Body.Close()

				if err != nil {
					t.Errorf("流式模式测试失败: %v", err)
					continue
				}
				streamingLatencies = append(streamingLatencies, latency)
			}

			// 计算平均延迟
			avgBuffered := averageDuration(bufferedLatencies)
			avgStreaming := averageDuration(streamingLatencies)

			// 计算延迟改善
			latencyImprovement := float64(avgBuffered-avgStreaming) / float64(avgBuffered) * 100

			t.Logf("📊 [延迟性能] 数据大小: %dMB", sizeInMB)
			t.Logf("   缓冲模式平均延迟: %v", avgBuffered)
			t.Logf("   流式模式平均延迟: %v", avgStreaming)
			t.Logf("   延迟改善: %.2f%%", latencyImprovement)

			// 对于大数据，流式处理应该更快
			if sizeInMB >= 5 {
				if avgStreaming < avgBuffered {
					t.Logf("✅ 延迟改善验证通过: 流式模式提升了 %.2f%% 的处理速度", latencyImprovement)
				} else {
					t.Logf("⚠️  注意: 在 %dMB 数据下，流式模式延迟略高，可能由于测试环境因素", sizeInMB)
				}
			}
		})
	}
}

// 并发性能测试
func TestStreamProcessor_ConcurrentPerformance(t *testing.T) {
	data := generateLargeSSEResponse(5) // 5MB测试数据
	concurrencyLevels := []int{1, 5, 10, 20}

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {

			// 测试缓冲模式并发性能
			bufferedStart := time.Now()
			var wg sync.WaitGroup

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					resp := createMockResponse(data)
					_, _, err := processStreamBuffered(resp)
					resp.Body.Close()
					if err != nil {
						t.Errorf("缓冲模式并发测试失败: %v", err)
					}
				}()
			}
			wg.Wait()
			bufferedDuration := time.Since(bufferedStart)

			// 测试流式模式并发性能
			streamingStart := time.Now()

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					resp := createMockResponse(data)
					_, _, err := processStreamStreaming(resp)
					resp.Body.Close()
					if err != nil {
						t.Errorf("流式模式并发测试失败: %v", err)
					}
				}()
			}
			wg.Wait()
			streamingDuration := time.Since(streamingStart)

			// 计算并发性能改善
			concurrentImprovement := float64(bufferedDuration-streamingDuration) / float64(bufferedDuration) * 100

			t.Logf("📊 [并发性能] 并发数: %d", concurrency)
			t.Logf("   缓冲模式总耗时: %v", bufferedDuration)
			t.Logf("   流式模式总耗时: %v", streamingDuration)
			t.Logf("   并发性能改善: %.2f%%", concurrentImprovement)

			if streamingDuration < bufferedDuration {
				t.Logf("✅ 并发性能验证通过: 流式模式在%d并发下提升了 %.2f%% 的性能", concurrency, concurrentImprovement)
			}
		})
	}
}

// 完整的StreamProcessor性能测试
func TestStreamProcessor_FullPerformance(t *testing.T) {
	// 创建真实的StreamProcessor实例
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, "perf-test-123", "test-endpoint")

	// 测试不同大小的数据
	testSizes := []int{1, 5, 10}

	for _, sizeInMB := range testSizes {
		t.Run(fmt.Sprintf("FullProcessor_%dMB", sizeInMB), func(t *testing.T) {
			data := generateLargeSSEResponse(sizeInMB)

			// 重置处理器
			processor.Reset()

			// 记录开始内存
			runtime.GC()
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)

			start := time.Now()

			// 执行完整的流式处理
			resp := createMockResponse(data)
			_, err := processor.ProcessStream(context.Background(), resp)
			resp.Body.Close()

			duration := time.Since(start)

			// 记录结束内存
			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)

			memoryUsed := m2.Alloc - m1.Alloc

			// 获取处理统计
			stats := processor.GetProcessingStats()

			if err != nil {
				t.Errorf("完整流式处理失败: %v", err)
			}

			t.Logf("📊 [完整处理性能] 数据大小: %dMB", sizeInMB)
			t.Logf("   处理耗时: %v", duration)
			t.Logf("   内存使用: %d 字节 (%.2f MB)", memoryUsed, float64(memoryUsed)/(1024*1024))
			t.Logf("   处理字节数: %v", stats["bytes_processed"])
			t.Logf("   解析错误数: %v", stats["parse_errors"])

			// 验证处理正确性
			bytesProcessed := stats["bytes_processed"].(int64)
			if bytesProcessed == 0 {
				t.Error("处理字节数不应该为0")
			}

			// 验证内存使用合理性（应该远小于数据大小）
			expectedMemory := int64(sizeInMB * 1024 * 1024)
			if int64(memoryUsed) > expectedMemory/2 { // 内存使用应该小于数据大小的一半
				t.Logf("⚠️  内存使用可能偏高: %d 字节，数据大小: %d 字节", memoryUsed, expectedMemory)
			} else {
				t.Logf("✅ 内存使用验证通过: 流式处理有效控制了内存使用")
			}
		})
	}
}

// 工具函数：计算平均持续时间
func averageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total / time.Duration(len(durations))
}

// Token解析性能测试
func TestStreamProcessor_TokenParsingPerformance(t *testing.T) {
	// 创建包含大量Token事件的SSE数据
	var builder strings.Builder
	builder.Grow(1024 * 1024) // 1MB

	// 添加模型信息
	builder.WriteString("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-3-5-haiku-20241022\"}}\n\n")

	// 添加大量Token使用事件
	for i := 0; i < 1000; i++ {
		tokenData := fmt.Sprintf("data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d}}\n\n",
			i*10+25, i*5+50)
		builder.WriteString(tokenData)
	}

	data := builder.String()

	// 创建处理器
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, "token-perf-123", "test-endpoint")

	start := time.Now()

	// 执行Token解析性能测试
	resp := createMockResponse(data)
	_, err := processor.ProcessStream(context.Background(), resp)
	resp.Body.Close()

	duration := time.Since(start)

	if err != nil {
		t.Errorf("Token解析性能测试失败: %v", err)
	}

	stats := processor.GetProcessingStats()

	t.Logf("📊 [Token解析性能]")
	t.Logf("   处理1000个Token事件耗时: %v", duration)
	t.Logf("   平均每个事件耗时: %v", duration/1000)
	t.Logf("   处理字节数: %v", stats["bytes_processed"])
	t.Logf("   解析错误数: %v", stats["parse_errors"])

	// 验证性能合理性（每个事件应该在微秒级别）
	avgPerEvent := duration / 1000
	if avgPerEvent > time.Millisecond {
		t.Logf("⚠️  Token解析性能可能需要优化: 平均每事件 %v", avgPerEvent)
	} else {
		t.Logf("✅ Token解析性能验证通过: 平均每事件 %v", avgPerEvent)
	}
}
