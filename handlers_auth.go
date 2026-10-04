package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// ---------- GOOGLE ----------

func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state-google")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	ctx := c.Request.Context()
	code := c.Query("code")

	token, err := googleOauthConfig.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://www.googleapis.com/oauth2/v2/userinfo?access_token="+token.AccessToken, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp, err := http.DefaultClient.Do(req)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "google_no_email"})
		return
	}
	if name == "" {
		name = email
	}

	name = truncateRunes(name, MaxUsernameLen)

	var (
		userID     int
		dbUsername string
		isPremium  bool
	)

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, google_id, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
		SET google_id = EXCLUDED.google_id,
			avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url)
		RETURNING id, username, is_premium
	`, name, email, googleID, picture).Scan(&userID, &dbUsername, &isPremium)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	auditLog(c, userID, "login", "user", userID)

	jwtToken, err := generateJWT(userID, dbUsername, isPremium)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}

// ---------- GITHUB ----------

func handleGithubLogin(c *gin.Context) {
	url := githubOauthConfig.AuthCodeURL("state-github")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGithubCallback(c *gin.Context) {
	ctx := c.Request.Context()
	code := c.Query("code")

	token, err := githubOauthConfig.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	client := &http.Client{}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
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
		emailReq, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
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

	login = truncateRunes(login, MaxUsernameLen)

	var (
		userID     int
		dbUsername string
		isPremium  bool
	)
	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, github_id, avatar_url, bio)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email) DO UPDATE
		SET github_id = EXCLUDED.github_id,
			avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url)
		RETURNING id, username, is_premium
	`, login, email, githubID, avatar, bio).Scan(&userID, &dbUsername, &isPremium)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	auditLog(c, userID, "login", "user", userID)

	jwtToken, err := generateJWT(userID, dbUsername, isPremium)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	frontend := os.Getenv("FRONTEND_URL")
	c.Redirect(http.StatusTemporaryRedirect, frontend+"/auth/callback?token="+jwtToken)
}
