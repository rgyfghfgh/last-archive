package functions

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"spider/db"
	"spider/models"
)

func (c *Crawler) CrawlPage(websiteUrl string) error {
	log.Printf("Crawling: %s", websiteUrl)
	pagesCrawled++
	c.httpClient.Transport = ProxyTransport()
	defer c.addToSeen(websiteUrl)

	if _, visited := c.VisitedUrls[websiteUrl]; visited {
		log.Printf("%s already visited, skipping", websiteUrl)
		appendLog(fmt.Sprintf("%s already visited, skipping", websiteUrl))
		return nil
	}

	if c.shouldSkipURL(websiteUrl) {
		log.Printf("Skipping non-HTML content: %s", websiteUrl)
		return nil
	}

	parsedURL, err := url.Parse(websiteUrl)
	if err != nil {
		log.Printf("Failed to parse URL %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	protocol := parsedURL.Scheme + "://"
	domain := parsedURL.Host
	targetPath := parsedURL.Path

	if err := c.CheckingRobotsRules((protocol + domain), targetPath); err != nil {
		log.Printf("Robots.txt blocked %s: %v", websiteUrl, err)
		appendLog(fmt.Sprintf("Robots.txt blocked %s: %v", websiteUrl, err))
		return fmt.Errorf("robots.txt blocked: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Ctx, 30*time.Second)
	defer cancel()

	startTime := time.Now()

	request, err := http.NewRequestWithContext(ctx, "GET", websiteUrl, nil)
	if err != nil {
		log.Printf("Failed to create request for %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(request)
	responseTime := time.Since(startTime)

	if err != nil {
		log.Printf("Failed to fetch %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Skipped %s, status: %d", websiteUrl, resp.StatusCode)
		return fmt.Errorf("non-200 status code: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !c.isHTMLContent(contentType) {
		log.Printf("Skipping non-HTML content: %s (Content-Type: %s)", websiteUrl, contentType)
		return nil
	}

	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)
	bodyData, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Printf("Failed to read body for %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to read body: %w", err)
	}

	pageData, err := c.extractPageData(string(bodyData), websiteUrl, domain, protocol, resp, responseTime)
	if err != nil {
		log.Printf("Failed to extract page data for %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to extract page data: %w", err)
	}

	if len(pageData.MainContent) < minContentLength {
		log.Printf("Skipping %s: content too short (%d characters, minimum %d)", websiteUrl, len(pageData.MainContent), minContentLength)
		return nil
	}

	if err := c.storePageDataWithRetry(pageData, 3); err != nil {
		log.Printf("Failed to store page data after retries for %s: %v", websiteUrl, err)
		return fmt.Errorf("failed to store page data: %w", err)
	}

	log.Printf("Successfully processed %s, found %d outbound links, %d PDFs", websiteUrl, len(pageData.OutboundLinks), len(pageData.PDFs))
	return nil
}

func (c *Crawler) storePageDataWithRetry(pageData *models.PageData, maxRetries int) error {
	pageData.MainContent = c.cleanContent(pageData.MainContent)

	log.Printf("Storing page data for: %s (Title: %s, Words: %d, Links: %d, PDFs: %d)",
		pageData.URL, pageData.Title, pageData.WordCount, len(pageData.OutboundLinks), len(pageData.PDFs))

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-c.Ctx.Done():
			return fmt.Errorf("operation cancelled")
		default:
		}

		if err := db.GetSQLiteHandler().HealthCheck(); err != nil {
			log.Printf("Database health check failed before storing %s (attempt %d): %v", pageData.URL, attempt, err)
			lastErr = err

			if attempt < maxRetries {
				backoffTime := time.Duration(attempt*attempt) * time.Second
				log.Printf("Retrying in %v...", backoffTime)

				select {
				case <-c.Ctx.Done():
					return fmt.Errorf("operation cancelled during backoff")
				case <-time.After(backoffTime):
					continue
				}
			}
			continue
		}

		for len(pageData.URL) > 0 && pageData.URL[len(pageData.URL)-1] == '/' {
			pageData.URL = pageData.URL[0:(len(pageData.URL) - 1)]
		}

		err := db.GetSQLiteHandler().UpsertPageData(*pageData)
		if err == nil {
			log.Printf("Successfully stored page data for: %s", pageData.URL)
			appendLog(fmt.Sprintf("Successfully stored page data for: %s", pageData.URL))
			return nil
		}

		lastErr = err
		log.Printf("Attempt %d/%d failed to store page data for %s: %v", attempt, maxRetries, pageData.URL, err)

		if attempt < maxRetries {
			backoffTime := time.Duration(attempt*attempt) * time.Second
			select {
			case <-c.Ctx.Done():
				return fmt.Errorf("operation cancelled during backoff")
			case <-time.After(backoffTime):
				continue
			}
		}
	}

	return fmt.Errorf("failed to store page data after %d attempts: %w", maxRetries, lastErr)
}

func (c *Crawler) shouldSkipURL(url string) bool {
	binaryExtensions := []string{
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg",
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".zip", ".rar", ".tar", ".gz", ".7z",
		".mp3", ".mp4", ".wav", ".avi", ".mov", ".wmv",
		".css", ".js", ".ico", ".xml", ".json", ".php",
	}

	lowerURL := strings.ToLower(url)
	for _, ext := range binaryExtensions {
		if strings.HasSuffix(lowerURL, ext) {
			return true
		}
	}

	if strings.Contains(lowerURL, "download=") ||
		strings.Contains(lowerURL, "attachment=") ||
		strings.Contains(lowerURL, "export=") {
		return true
	}

	return false
}

func (c *Crawler) isHTMLContent(contentType string) bool {
	if contentType == "" {
		return true
	}

	lowerType := strings.ToLower(contentType)
	htmlTypes := []string{
		"text/html",
		"application/xhtml+xml",
		"text/plain",
	}

	for _, htmlType := range htmlTypes {
		if strings.Contains(lowerType, htmlType) {
			return true
		}
	}

	return false
}

func (c *Crawler) isAllowedOrigin(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return c.AllowedHosts[parsedURL.Host]
}
