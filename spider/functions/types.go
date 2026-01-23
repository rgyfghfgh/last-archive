package functions

import (
	"context"
	"encoding/xml"
	"net/http"
	"os"
	"sync"
	"time"

	"spider/models"

	"github.com/temoto/robotstxt"
)

// Sitemap structures for XML parsing
type Sitemap struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []SitemapURL `xml:"url"`
}

type SitemapIndex struct {
	XMLName  xml.Name           `xml:"sitemapindex"`
	Sitemaps []SitemapReference `xml:"sitemap"`
}

type SitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

type SitemapReference struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

// Crawler holds the state and configuration for web crawling
type Crawler struct {
	BaseDomain    string
	AllowedHosts  map[string]bool // For tracking all allowed origins
	LinksQueue    *[]models.Link
	VisitedUrls   map[string]struct{}
	QueuedUrls    map[string]bool
	ProcessedPDFs map[string]bool // Track processed PDFs to avoid duplicates
	Mu            *sync.Mutex
	Ctx           context.Context
	cancel        context.CancelFunc
	shutdownChan  chan os.Signal
	httpClient    *http.Client
	imagesDir     string
	pdfsDir       string
	embeddingFunc func(string) ([]float32, error)
}

var (
	robotsCache      = make(map[string]*robotstxt.RobotsData)
	robotsCacheMu    sync.RWMutex
	transport        = ProxyTransport()
	timesleep        = 2 * time.Second
	userAgent        = "TheLastArchiveBot/1.0"
	pagesCrawled     = 0
	minContentLength = 500
)
