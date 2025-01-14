package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RateLimiterConfig struct {
	RPS            float64
	Burst          int
	ExpirationTime time.Duration
	LimitType      string
	KeyFunc        func(*gin.Context) string
}

type ClientTracker struct {
	limiter      *rate.Limiter
	lastSeen     time.Time
	totalRequest int64
}

type RateLimiter struct {
	config    RateLimiterConfig
	clients   map[string]*ClientTracker
	mu        sync.RWMutex
	cleanupTk *time.Ticker
}

func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if config.RPS == 0 {
		config.RPS = 10
	}
	if config.Burst == 0 {
		config.Burst = 20
	}
	if config.ExpirationTime == 0 {
		config.ExpirationTime = 1 * time.Hour
	}
	if config.KeyFunc == nil {
		config.KeyFunc = defaultKeyFunc
	}

	rl := &RateLimiter{
		config:    config,
		clients:   make(map[string]*ClientTracker),
		cleanupTk: time.NewTicker(config.ExpirationTime),
	}

	go rl.cleanup()

	return rl
}

func defaultKeyFunc(c *gin.Context) string {
	return c.ClientIP()
}

func (rl *RateLimiter) cleanup() {
	for range rl.cleanupTk.C {
		rl.mu.Lock()
		for key, client := range rl.clients {
			if time.Since(client.lastSeen) > rl.config.ExpirationTime {
				delete(rl.clients, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) getClientLimiter(key string) *ClientTracker {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if client, exists := rl.clients[key]; exists {
		client.lastSeen = time.Now()
		return client
	}

	client := &ClientTracker{
		limiter:  rate.NewLimiter(rate.Limit(rl.config.RPS), rl.config.Burst),
		lastSeen: time.Now(),
	}
	rl.clients[key] = client
	return client
}

func RateLimit(config RateLimiterConfig) gin.HandlerFunc {
	rateLimiter := NewRateLimiter(config)

	return func(c *gin.Context) {
		key := config.KeyFunc(c)
		client := rateLimiter.getClientLimiter(key)

		client.totalRequest++
		if !client.limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"rate": gin.H{
					"requests_per_second": config.RPS,
					"burst":               config.Burst,
					"total_requests":      client.totalRequest,
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func APIRateLimit() gin.HandlerFunc {
	config := RateLimiterConfig{
		RPS:            10,
		Burst:          20,
		ExpirationTime: 1 * time.Hour,
		LimitType:      "api",
		KeyFunc: func(c *gin.Context) string {
			if key := c.GetHeader("X-API-Key"); key != "" {
				return key
			}
			return c.Query("api_key")
		},
	}
	return RateLimit(config)
}

func IPRateLimit() gin.HandlerFunc {
	config := RateLimiterConfig{
		RPS:            5,
		Burst:          10,
		ExpirationTime: 1 * time.Hour,
		LimitType:      "ip",
		KeyFunc:        defaultKeyFunc,
	}
	return RateLimit(config)
}

func SectorAPIRateLimit() gin.HandlerFunc {
	config := RateLimiterConfig{
		RPS:            2,
		Burst:          5,
		ExpirationTime: 1 * time.Hour,
		LimitType:      "sector_api",
		KeyFunc: func(c *gin.Context) string {
			sector := c.Query("sector")
			if sector == "" {
				sector = "all"
			}
			return c.ClientIP() + ":" + sector
		},
	}
	return RateLimit(config)
}
