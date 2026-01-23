package functions

import (
	"fmt"
	"log"

	"spider/models"
	"spider/utils"
)

func (c *Crawler) safeDequeue() (models.Link, bool) {
	if c == nil || c.Mu == nil || c.LinksQueue == nil || c.QueuedUrls == nil {
		log.Printf("ERROR: Crawler or its components are nil in safeDequeue")
		return models.Link{}, false
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	if len(*c.LinksQueue) == 0 {
		return models.Link{}, false
	}

	link, newQueue, err := utils.Dequeue(*c.LinksQueue)
	if err != nil {
		log.Printf("ERROR: Failed to dequeue: %v", err)
		return models.Link{}, false
	}

	*c.LinksQueue = newQueue
	delete(c.QueuedUrls, link.URL)

	log.Printf("Dequeued: %s, Queue size: %d", link.URL, len(*c.LinksQueue))
	return link, true
}

func (c *Crawler) safeEnqueue(link models.Link) {
	if c == nil || c.Mu == nil || c.LinksQueue == nil || c.QueuedUrls == nil || c.VisitedUrls == nil {
		log.Printf("ERROR: Crawler components are nil")
		return
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	if _, exists := c.QueuedUrls[link.URL]; exists {
		return
	}

	if _, visited := c.VisitedUrls[link.URL]; visited {
		return
	}

	if !c.isAllowedOrigin(link.URL) {
		return
	}

	*c.LinksQueue = utils.Enqueue(*c.LinksQueue, link)
	c.QueuedUrls[link.URL] = true

	log.Printf("Enqueued: %s, Queue size: %d", link.URL, len(*c.LinksQueue))
	appendLog(fmt.Sprintf("Enqueued: %s, Queue size: %d", link.URL, len(*c.LinksQueue)))
}

func (c *Crawler) addToSeen(url string) {
	if c == nil || c.Mu == nil || c.VisitedUrls == nil {
		log.Printf("ERROR: Cannot add to seen - crawler components are nil")
		return
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.VisitedUrls[url] = struct{}{}
}
