package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// ---------- GOOGLE ----------

// Kullanıcıyı Google giriş sayfasına yönlendirir.
func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state-google")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// Google'dan dönen code'u token'a çevirir, kullanıcıyı DB'ye yazar, JWT ile frontend'e yollar.
func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := googleOauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	googleID := fmt.Sprintf("%v", info["id"])
	email, _ := info["email"].(string)
	name, _ := info["name"].(string)
	picture, _ := info["picture"].(string)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Google did not return email"})
		return
	}
	if name == "" {
		name = email
	}

	// DB sınırı: username VARCHAR(30) — UTF-8 güvenli truncate
	name = truncateRunes(name, MaxUsernameLen)

	var (
		userID     int
		dbUsername string
		dbAvatar   string
	)
	err = db.QueryRow(context.Background(), `
		INSERT INTO users (username, email, google_id, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
		SET google_id = EXCLUDED.google_id,
		    avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url)
		RETURNING id, username, COALESCE(avatar_url, '')
	`, name, email, googleID, picture).Scan(&userID, &dbUsername, &dbAvatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// JWT'ye DB'deki GERÇEK username ve avatar yazılsın
	jwtToken, err := generateJWT(userID, email, dbUsername, dbAvatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}

// ---------- GITHUB ----------

// Kullanıcıyı GitHub giriş sayfasına yönlendirir.
func handleGithubLogin(c *gin.Context) {
	url := githubOauthConfig.AuthCodeURL("state-github")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GitHub'dan dönen code'u token'a çevirir, kullanıcıyı DB'ye yazar, JWT ile frontend'e yollar.
func handleGithubCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := githubOauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	client := &http.Client{}

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	login, _ := info["login"].(string)
	avatar, _ := info["avatar_url"].(string)
	bio, _ := info["bio"].(string)

	var githubID int
	if v, ok := info["id"].(float64); ok {
		githubID = int(v)
	}

	email, _ := info["email"].(string)
	if email == "" {
		emailReq, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
		emailReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
		emailReq.Header.Set("Accept", "application/vnd.github+json")

		emailResp, err := client.Do(emailReq)
		if err == nil {
			defer emailResp.Body.Close()
			emailBody, _ := io.ReadAll(emailResp.Body)
			var emails []map[string]interface{}
			json.Unmarshal(emailBody, &emails)
			for _, e := range emails {
				if primary, _ := e["primary"].(bool); primary {
					email, _ = e["email"].(string)
					break
				}
			}
		}
	}

	if email == "" {
		email = fmt.Sprintf("%s@users.noreply.github.com", login)
	}
	if login == "" {
		login = fmt.Sprintf("github_%d", githubID)
	}

	// DB sınırı: username VARCHAR(30) — UTF-8 güvenli truncate
	login = truncateRunes(login, MaxUsernameLen)

	var (
		userID     int
		dbUsername string
		dbAvatar   string
	)
	err = db.QueryRow(context.Background(), `
		INSERT INTO users (username, email, github_id, avatar_url, bio)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email) DO UPDATE
		SET github_id = EXCLUDED.github_id,
			avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url)
		RETURNING id, username, COALESCE(avatar_url, '')
	`, login, email, githubID, avatar, bio).Scan(&userID, &dbUsername, &dbAvatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// JWT'ye DB'deki GERÇEK username ve avatar yazılsın
	jwtToken, err := generateJWT(userID, email, dbUsername, dbAvatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}
