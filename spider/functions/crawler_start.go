package functions

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"spider/models"
)

func (c *Crawler) Start(workerCount int, seedUrls ...string) {
	if c == nil {
		log.Fatal("Crawler is nil, can't run")
		return
	}
	if c.Mu == nil {
		log.Fatal("Mutex not set, stop")
		return
	}

	if len(seedUrls) == 0 {
		logText := "No seed URLs, nothing to do"
		log.Println(logText)
		appendLog(logText)
		return
	}

	// set main domain and allowed hosts
	for _, seedURL := range seedUrls {
		if parsedURL, err := url.Parse(seedURL); err == nil {
			c.AllowedHosts[parsedURL.Host] = true
			if c.BaseDomain == "" {
				c.BaseDomain = parsedURL.Host
			}
			log.Printf("Allowed host added: %s", parsedURL.Host)
			appendLog(fmt.Sprintf("Allowed host added: %s", parsedURL.Host))
		}
	}

	log.Printf("Base domain is: %s", c.BaseDomain)
	appendLog(fmt.Sprintf("Base domain is: %s", c.BaseDomain))

	// first priority: push seed URLs to queue
	log.Println("Pushing seed URLs first...")
	appendLog("Pushing seed URLs first...")

	for i, seedURL := range seedUrls {
		_, err := url.Parse(seedURL)
		if err != nil {
			log.Printf("Can't parse seed URL %s: %v, skip", seedURL, err)
			appendLog(fmt.Sprintf("Can't parse seed URL %s: %v, skip", seedURL, err))
			continue
		}

		log.Printf("Queueing seed URL %d/%d: %s", i+1, len(seedUrls), seedURL)
		appendLog(fmt.Sprintf("Queueing seed URL %d/%d: %s", i+1, len(seedUrls), seedURL))

		c.safeEnqueue(models.Link{URL: seedURL})
	}

	c.Mu.Lock()
	initialQueueSize := len(*c.LinksQueue)
	c.Mu.Unlock()

	log.Printf("%d seed URLs added to queue", initialQueueSize)
	appendLog(fmt.Sprintf("%d seed URLs added to queue", initialQueueSize))

	// second priority: try sitemaps in background
	go func() {
		log.Println("Looking for sitemaps...")
		appendLog("Looking for sitemaps...")

		for i, seedURL := range seedUrls {
			parsedURL, err := url.Parse(seedURL)
			if err != nil {
				continue
			}

			baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
			log.Printf("Checking sitemap for %s (%d/%d)", baseURL, i+1, len(seedUrls))
			appendLog(fmt.Sprintf("Checking sitemap for %s (%d/%d)", baseURL, i+1, len(seedUrls)))

			if err := c.crawlFromSitemap(baseURL); err != nil {
				log.Printf("Sitemap failed for %s: %v", baseURL, err)
				appendLog(fmt.Sprintf("Sitemap failed for %s: %v", baseURL, err))
			} else {
				c.Mu.Lock()
				currentSize := len(*c.LinksQueue)
				c.Mu.Unlock()

				added := currentSize - initialQueueSize
				log.Printf("Sitemap for %s gave %d new links", baseURL, added)
				appendLog(fmt.Sprintf("Sitemap for %s gave %d new links", baseURL, added))
			}
		}

		log.Println("Sitemap check done")
		appendLog("Sitemap check done")
	}()

	c.Mu.Lock()
	queueSize := len(*c.LinksQueue)
	c.Mu.Unlock()

	if queueSize == 0 {
		log.Println("Queue is empty, nothing to crawl")
		appendLog("Queue is empty, nothing to crawl")
		return
	}

	log.Printf("Starting crawl, %d URLs in queue", queueSize)
	appendLog(fmt.Sprintf("Starting crawl, %d URLs in queue", queueSize))

	var wg sync.WaitGroup

	go c.monitorShutdown()

	// start workers
	for i := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer log.Printf("Worker %d done", id)

			consecutiveEmpty := 0
			maxEmpty := 10

			for {
				select {
				case <-c.Ctx.Done():
					log.Printf("Worker %d got stop signal", id)
					appendLog(fmt.Sprintf("Worker %d got stop signal", id))
					return
				default:
				}

				link, ok := c.safeDequeue()
				if !ok {
					consecutiveEmpty++
					if consecutiveEmpty >= maxEmpty {
						log.Printf("Worker %d: nothing to do, exit after %d tries", id, maxEmpty)
						return
					}

					log.Printf("Worker %d: queue empty, wait... (%d/%d)", id, consecutiveEmpty, maxEmpty)

					select {
					case <-c.Ctx.Done():
						return
					case <-time.After(5 * time.Second):
						continue
					}
				}

				consecutiveEmpty = 0

				log.Printf("Worker %d: crawling %s", id, link.URL)
				if err := c.CrawlPage(link.URL); err != nil {
					log.Printf("Worker %d: error with %s: %v", id, link.URL, err)
				}

				select {
				case <-c.Ctx.Done():
					return
				case <-time.After(timesleep):
				}
			}
		}(i)
	}

	wg.Wait()
	log.Printf("All workers done, pages crawled: %d", pagesCrawled)
	appendLog(fmt.Sprintf("All workers done, pages crawled: %d", pagesCrawled))
}
