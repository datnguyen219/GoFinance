// middleware/rate_limit_test.go

package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Basic Rate Limiting", func(t *testing.T) {
		router := gin.New()
		router.Use(IPRateLimit())
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "success")
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"

		for i := 0; i < 6; i++ {
			router.ServeHTTP(w, req)
			if i < 5 {
				assert.Equal(t, http.StatusOK, w.Code)
			} else {
				assert.Equal(t, http.StatusTooManyRequests, w.Code)
			}
			w = httptest.NewRecorder()
		}
	})

	t.Run("Burst Handling", func(t *testing.T) {
		router := gin.New()
		router.Use(IPRateLimit())
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "success")
		})

		var wg sync.WaitGroup
		results := make([]int, 15)

		for i := 0; i < 15; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "1.2.3.4:1234"
				router.ServeHTTP(w, req)
				results[index] = w.Code
			}(i)
		}
		wg.Wait()

		ok := 0
		tooMany := 0
		for _, code := range results {
			if code == http.StatusOK {
				ok++
			} else if code == http.StatusTooManyRequests {
				tooMany++
			}
		}

		assert.Equal(t, 10, ok)
		assert.Equal(t, 5, tooMany)
	})

	t.Run("Recovery After Wait", func(t *testing.T) {
		router := gin.New()
		router.Use(IPRateLimit())
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "success")
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"

		// Exhaust the limit
		for i := 0; i < 5; i++ {
			router.ServeHTTP(w, req)
			w = httptest.NewRecorder()
		}

		// Wait for rate limit to recover
		time.Sleep(1 * time.Second)

		// Should succeed again
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
