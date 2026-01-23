package functions

import (
	"fmt"
	"net/http"

	"github.com/temoto/robotstxt"
)

func (c *Crawler) CheckingRobotsRules(domain string, targetPath string) error {
	robotsCacheMu.RLock()
	robotsData, exists := robotsCache[domain]
	robotsCacheMu.RUnlock()

	if exists {
		group := robotsData.FindGroup("*")
		if !group.Test(targetPath) {
			return fmt.Errorf("blocked by robots.txt: %s", targetPath)
		}
		return nil
	}

	resp, err := http.Get(domain + "/robots.txt")
	if err != nil {
		return fmt.Errorf("failed to fetch robots.txt: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	robotsData, err = robotstxt.FromResponse(resp)
	if err != nil {
		return fmt.Errorf("failed to parse robots.txt: %v", err)
	}

	group := robotsData.FindGroup("*")
	canFetch := group.Test(targetPath)

	if !canFetch {
		return fmt.Errorf("not allowed to fetch %s (blocked by robots.txt)", targetPath)
	}

	robotsCacheMu.Lock()
	robotsCache[domain] = robotsData
	robotsCacheMu.Unlock()

	return nil
}
