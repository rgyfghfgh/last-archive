package functions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *Crawler) downloadImage(imageURL, pageURL string) (string, error) {
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return "", fmt.Errorf("invalid image URL: %w", err)
	}

	hash := sha256.Sum256([]byte(imageURL))
	hashStr := hex.EncodeToString(hash[:16])

	ext := filepath.Ext(parsedURL.Path)
	ext = strings.ToLower(ext)
	ext = strings.TrimSpace(ext)

	validExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true, ".svg": true, ".bmp": true, ".ico": true,
	}

	if ext == "" || len(ext) > 5 || !validExts[ext] {
		ext = ".jpg"
	}

	filename := hashStr + ext
	imagePath := filepath.Join(c.imagesDir, filename)

	if _, err := os.Stat(imagePath); err == nil {
		log.Printf("Image already exists: %s", imagePath)
		return imagePath, nil
	}

	ctx, cancel := context.WithTimeout(c.Ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", pageURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("not an image content type: %s", contentType)
	}

	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)

	tempPath := imagePath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	_, err = io.Copy(file, limitedReader)
	file.Close()

	if err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	if err := os.Rename(tempPath, imagePath); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to rename image: %w", err)
	}

	log.Printf("Downloaded image: %s -> %s", imageURL, imagePath)
	return imagePath, nil
}
