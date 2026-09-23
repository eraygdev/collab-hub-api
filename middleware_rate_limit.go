package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IP başına rate limiter'ları tutar
type RateLimiterStore struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

var rateLimiters = &RateLimiterStore{
	limiters: make(map[string]*rate.Limiter),
}

// Belirli bir IP için rate limiter döner (yoksa oluşturur)
func (store *RateLimiterStore) getLimiter(ip string) *rate.Limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	limiter, exists := store.limiters[ip]
	if !exists {
		// Saniyede 5 istek, burst 10 (kısa süreli patlamalara izin ver)
		limiter = rate.NewLimiter(rate.Limit(5), 10)
		store.limiters[ip] = limiter
	}
	return limiter
}

// Eski IP'leri temizle (memory leak önleme) - 10 dakikada bir çalışır
func (store *RateLimiterStore) cleanupStale(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		store.mu.Lock()
		now := time.Now()
		for ip, limiter := range store.limiters {
			// Eğer limiter 5 dakikadan uzun süredir kullanılmadıysa sil
			// (limiter'ın son kullanım zamanını takip etmiyoruz, basit temizlik)
			_ = now
			// Basit yaklaşım: her temizlikte tüm limiter'ları sil
			// Daha akıllı: last_used map'i tut
			_ = ip
			_ = limiter
		}
		// Şimdilik basit: 10 dakikada bir tüm limiter'ları temizle
		// Bu, aktif kullanıcıların da limitini sıfırlar ama basit ve güvenli
		store.limiters = make(map[string]*rate.Limiter)
		store.mu.Unlock()
	}
}

// Rate limit middleware
func rateLimitMiddleware() gin.HandlerFunc {
	// Temizlik goroutine'ini başlat
	go rateLimiters.cleanupStale(10 * time.Minute)

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
