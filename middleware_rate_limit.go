package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IP başına rate limiter ve son görülme zamanını tutar
type RateLimiterStore struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	lastSeen map[string]time.Time
}

var rateLimiters = &RateLimiterStore{
	limiters: make(map[string]*rate.Limiter),
	lastSeen: make(map[string]time.Time),
}

// Belirli bir IP için rate limiter döner (yoksa oluşturur) ve son görülme zamanını günceller.
func (store *RateLimiterStore) getLimiter(ip string) *rate.Limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	limiter, exists := store.limiters[ip]
	if !exists {
		// Saniyede 5 istek, burst 10
		limiter = rate.NewLimiter(rate.Limit(5), 10)
		store.limiters[ip] = limiter
	}

	// Her istekte son görülme zamanını güncelle
	store.lastSeen[ip] = time.Now()

	return limiter
}

// Pasif IP'leri temizle (memory leak önleme).
// Sadece belirtilen süreden uzun süredir istek atmayan IP'ler silinir.
// Aktif kullanıcıların limiter'ı korunur.
func (store *RateLimiterStore) cleanupStale(interval, maxIdle time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		store.mu.Lock()
		now := time.Now()
		removed := 0

		for ip, last := range store.lastSeen {
			if now.Sub(last) > maxIdle {
				delete(store.limiters, ip)
				delete(store.lastSeen, ip)
				removed++
			}
		}

		store.mu.Unlock()

		if removed > 0 {
			// İsteğe bağlı log — istersen kaldırabilirsin
			// log.Printf("[RATE LIMIT] %d pasif IP temizlendi", removed)
		}
	}
}

// Rate limit middleware
func rateLimitMiddleware() gin.HandlerFunc {
	// Temizlik goroutine'ini başlat:
	// - Her 1 dakikada bir kontrol et
	// - 5 dakikadan uzun süredir istek atmayan IP'leri sil
	go rateLimiters.cleanupStale(1*time.Minute, 5*time.Minute)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		limiter := rateLimiters.getLimiter(ip)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Çok fazla istek gönderdiniz. Lütfen biraz bekleyin.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
