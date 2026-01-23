package functions

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"

	"spider/models"
)

func (c *Crawler) crawlFromSitemap(baseURL string) error {
	sitemapURLs := []string{
		baseURL + "/sitemap.xml",
		baseURL + "/sitemap_index.xml",
		baseURL + "/sitemaps.xml",
	}

	for _, sitemapURL := range sitemapURLs {
		log.Printf("Trying to fetch sitemap from: %s", sitemapURL)

		resp, err := c.httpClient.Get(sitemapURL)
		if err != nil {
			log.Printf("Failed to fetch sitemap from %s: %v", sitemapURL, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Sitemap not found at %s, status: %d", sitemapURL, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Failed to read sitemap body from %s: %v", sitemapURL, err)
			continue
		}

		var sitemap Sitemap
		if err := xml.Unmarshal(body, &sitemap); err == nil && len(sitemap.URLs) > 0 {
			log.Printf("Found sitemap with %d URLs", len(sitemap.URLs))
			for _, url := range sitemap.URLs {
				if c.isAllowedOrigin(url.Loc) {
					c.safeEnqueue(models.Link{URL: url.Loc})
				}
			}
			return nil
		}

		var sitemapIndex SitemapIndex
		if err := xml.Unmarshal(body, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
			log.Printf("Found sitemap index with %d sitemaps", len(sitemapIndex.Sitemaps))
			for _, sitemapRef := range sitemapIndex.Sitemaps {
				if err := c.crawlFromSitemap(sitemapRef.Loc); err != nil {
					log.Printf("Failed to crawl nested sitemap %s: %v", sitemapRef.Loc, err)
				}
			}
			return nil
		}

		log.Printf("Failed to parse sitemap from %s", sitemapURL)
	}

	log.Printf("No valid sitemap found for %s, adding base URL to queue", baseURL)
	c.safeEnqueue(models.Link{URL: baseURL})
	return nil
}
