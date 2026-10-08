package app

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var (
	githubOauthConfig *oauth2.Config
	jwtSecret         []byte

	// stateStore — geçici OAuth state'lerini tutar (CSRF koruması)
	stateStore   = make(map[string]time.Time)
	stateStoreMu sync.Mutex
)

// generateOAuthState rastgele state üretir ve store'a ekler.
func generateOAuthState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	state := base64.URLEncoding.EncodeToString(b)
	stateStoreMu.Lock()
	stateStore[state] = time.Now().Add(10 * time.Minute)
	stateStoreMu.Unlock()
	return state
}

// consumeOAuthState state'i doğrular ve tek kullanımlık olarak tüketir.
func consumeOAuthState(state string) bool {
	if state == "" {
		return false
	}
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()
	exp, ok := stateStore[state]
	if !ok {
		return false
	}
	delete(stateStore, state)
	return time.Now().Before(exp)
}

// cleanupOAuthStates süresi geçmiş state'leri temizler.
func cleanupOAuthStates() {
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()
	now := time.Now()
	for s, exp := range stateStore {
		if now.After(exp) {
			delete(stateStore, s)
		}
	}
}

// InitOAuth GitHub OAuth yapılandırmasını .env'den okur.
func InitOAuth() {
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	githubOauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_OAUTH_REDIRECT_URL"),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}
}

// truncateRunes string'i rune bazlı keser (multi-byte karakterleri bozmaz).
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// generateJWT 7 gün geçerli imzalı JWT üretir.
// Email kasıtlı olarak çıkarıldı — privacy.
func generateJWT(userID int, username string, isPremium bool) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"is_premium": isPremium,
		"exp":        time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// authMiddleware JWT'yi doğrular, user_id ve username'i context'e koyar.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_token"})
			c.Abort()
			return
		}

		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_claims"})
			c.Abort()
			return
		}

		uid, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_user_id"})
			c.Abort()
			return
		}
		c.Set("user_id", int(uid))

		if username, ok := claims["username"].(string); ok {
			c.Set("username", username)
		}

		c.Next()
	}
}

// authMiddlewareOptional token varsa doğrular, yoksa misafir olarak devam eder.
func authMiddlewareOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.Set("user_id", 0)
			c.Next()
			return
		}

		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.Set("user_id", 0)
			c.Next()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.Set("user_id", 0)
			c.Next()
			return
		}

		if uid, ok := claims["user_id"].(float64); ok {
			c.Set("user_id", int(uid))
		} else {
			c.Set("user_id", 0)
		}

		c.Next()
	}
}

// StartOAuthCleanup — süresi geçmiş OAuth state'lerini periyodik temizler.
// main.go'da goroutine olarak çağrılır.
func StartOAuthCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cleanupOAuthStates()
	}
}
