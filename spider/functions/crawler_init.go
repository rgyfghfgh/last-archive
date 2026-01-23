package functions

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"spider/models"
)

// NewCrawler setup and return new crawler instance
func NewCrawler(embeddingFunc func(string) ([]float32, error)) *Crawler {
	// base context used to control shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// listen for system stop signals
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	// queue for collected links
	linksQueue := make([]models.Link, 0)

	// shared http client for all requests
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	// make sure images folder exist
	imagesDir := "images"
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		log.Fatalf("failed to create images folder: %v", err)
	}

	// make sure pdfs folder exist
	pdfsDir := "pdfs"
	if err := os.MkdirAll(pdfsDir, 0755); err != nil {
		log.Fatalf("failed to create pdfs folder: %v", err)
	}

	// build crawler object
	crawler := &Crawler{
		AllowedHosts:  make(map[string]bool),
		QueuedUrls:    make(map[string]bool),
		VisitedUrls:   make(map[string]struct{}),
		ProcessedPDFs: make(map[string]bool),
		Mu:            &sync.Mutex{},
		LinksQueue:    &linksQueue,
		Ctx:           ctx,
		cancel:        cancel,
		shutdownChan:  shutdownChan,
		httpClient:    httpClient,
		imagesDir:     imagesDir,
		pdfsDir:       pdfsDir,
		embeddingFunc: embeddingFunc,
	}

	// sanity checks just in case
	if crawler.Mu == nil {
		log.Fatal("mutex was not initialized")
	}
	if crawler.LinksQueue == nil {
		log.Fatal("links queue was not initialized")
	}

	return crawler
}

func (c *Crawler) monitorShutdown() {
	// wait until shutdown signal arrive
	<-c.shutdownChan

	log.Println("shutdown signal received, stopping crawler...")
	c.cancel()

	// force exit if something still hanging
	go func() {
		time.Sleep(10 * time.Second)
		log.Println("shutdown timeout reached, force exiting now")
		os.Exit(1)
	}()
}
