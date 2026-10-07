package main

import (
	"fmt"
	"net/http"
	"time"
)

// checkGithubImageSize — GitHub raw görsel URL'ine HEAD isteği atıp
// Content-Length header'ından dosya boyutunu kontrol eder.
// Hata durumunda nil döner (fail-open — ağ hatası, rate limit vb.)
// Sadece kesin "çok büyük" durumunda error döner.
func checkGithubImageSize(imageURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("HEAD", imageURL, nil)
	if err != nil {
		return nil // fail-open
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil // fail-open
	}
	defer resp.Body.Close()

	// 200 değilse fail-open (404, 429 rate limit vb.)
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	size := resp.ContentLength
	// ContentLength -1 olabilir (bilinmiyorsa) → fail-open
	if size > 0 && size > MaxImageSizeBytes {
		return fmt.Errorf("image_too_large")
	}

	return nil
}
