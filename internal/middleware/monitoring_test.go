package middleware

import (
	"sync"
	"testing"
	"time"
)

func TestMonitoringMiddleware_ShouldBroadcastConcurrentAccess(t *testing.T) {
	middleware := NewMonitoringMiddleware(nil)

	const goroutines = 16
	const iterations = 2000

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				middleware.shouldBroadcast("connection_stats", time.Microsecond)
			}
		}()
	}

	close(start)
	wg.Wait()
}
