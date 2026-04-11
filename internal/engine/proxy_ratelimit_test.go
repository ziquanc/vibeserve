package engine

import (
	"testing"
	"time"
)

func TestProxyEngine_RateLimit(t *testing.T) {
	pe := &ProxyEngine{
		pending:     make(map[string]chan proxyResult),
		windowStart: time.Now(),
	}

	// Should allow up to proxyRateLimit
	for i := 0; i < proxyRateLimit; i++ {
		if err := pe.checkRateLimit(); err != nil {
			t.Fatalf("should allow generation %d, got error: %v", i+1, err)
		}
	}

	// Next one should be rejected
	if err := pe.checkRateLimit(); err == nil {
		t.Error("should reject generation after rate limit exceeded")
	}
}

func TestProxyEngine_RateLimitResetsAfterWindow(t *testing.T) {
	pe := &ProxyEngine{
		pending:     make(map[string]chan proxyResult),
		windowStart: time.Now().Add(-2 * proxyRateWindow), // window already expired
		genCount:    proxyRateLimit,                        // was at limit
	}

	// Should allow because window has expired
	if err := pe.checkRateLimit(); err != nil {
		t.Errorf("should allow after window reset, got error: %v", err)
	}
}
