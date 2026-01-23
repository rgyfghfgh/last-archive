package functions

import (
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"spider/models"
	"spider/utils"

	"golang.org/x/net/html"
)

func (c *Crawler) extractPageData(htmlContent, url, domain, protocol string, resp *http.Response, responseTime time.Duration) (*models.PageData, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	pageData := &models.PageData{
		URL:           url,
		Headings:      make(map[string][]string),
		ImageAlt:      make([]string, 0),
		Images:        make([]models.Image, 0),
		PDFs:          make([]models.PDFData, 0),
		LinkText:      make([]string, 0),
		OutboundLinks: make([]models.Link, 0),
		StatusCode:    resp.StatusCode,
		ResponseTime:  responseTime,
		ContentType:   resp.Header.Get("Content-Type"),
		CrawlDate:     time.Now(),
	}

	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if parsed, err := time.Parse(time.RFC1123, lastMod); err == nil {
			pageData.LastModified = parsed
		}
	}

	c.extractHTMLData(doc, pageData, domain, protocol)

	pageData.WordCount = len(strings.Fields(pageData.MainContent))

	return pageData, nil
}

func (c *Crawler) extractHTMLData(n *html.Node, pageData *models.PageData, domain, protocol string) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "title":
			titleText := c.extractTextContent(n)
			if titleText != "" && pageData.Title == "" {
				pageData.Title = titleText
			}

		case "meta":
			c.extractMetaData(n, pageData)

		case "h1", "h2", "h3", "h4", "h5", "h6":
			text := c.extractTextContent(n)
			if text != "" {
				pageData.Headings[n.Data] = append(pageData.Headings[n.Data], text)
			}

		case "img":
			alt := c.getAttributeValue(n, "alt")
			src := c.getAttributeValue(n, "src")

			if alt != "" {
				pageData.ImageAlt = append(pageData.ImageAlt, alt)
			}

			if src != "" {
				fullImageURL := c.constructFullURL(src, domain, protocol)
				if fullImageURL != "" && c.isAllowedOrigin(fullImageURL) {
					imagePath, err := c.downloadImage(fullImageURL, pageData.URL)
					if err != nil {
						log.Printf("Failed to download image %s: %v", fullImageURL, err)
					} else {
						pageData.Images = append(pageData.Images, models.Image{
							URL:  fullImageURL,
							Alt:  alt,
							Path: imagePath,
						})
					}
				}
			}

		case "a":
			c.extractLinkData(n, pageData, domain, protocol)

		case "link":
			rel := c.getAttributeValue(n, "rel")
			href := c.getAttributeValue(n, "href")

			if rel == "canonical" {
				pageData.Canonical = href
			} else if rel == "icon" || rel == "shortcut icon" || strings.Contains(rel, "icon") {
				if href != "" && pageData.Favicon == "" {
					faviconURL := c.constructFullURL(href, domain, protocol)
					pageData.Favicon = faviconURL
					log.Printf("Found favicon for %s: %s", pageData.URL, faviconURL)
				}
			}
		}
	}

	if n.Type == html.TextNode && c.isMainContent(n) {
		text := strings.TrimSpace(n.Data)
		if text != "" && len(text) > 3 {
			pageData.MainContent += " " + text
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.extractHTMLData(child, pageData, domain, protocol)
	}
}

func (c *Crawler) extractMetaData(n *html.Node, pageData *models.PageData) {
	name := c.getAttributeValue(n, "name")
	property := c.getAttributeValue(n, "property")
	content := c.getAttributeValue(n, "content")

	switch {
	case name == "description" || property == "og:description":
		if pageData.MetaDescription == "" {
			pageData.MetaDescription = content
		}
	case name == "keywords":
		pageData.MetaKeywords = content
	case name == "language" || property == "og:locale":
		pageData.Language = content
	case property == "og:title":
		if pageData.Title == "" {
			pageData.Title = content
		}
	}
}

func (c *Crawler) extractLinkData(linkNode *html.Node, pageData *models.PageData, domain, protocol string) {
	if rel := c.getAttributeValue(linkNode, "rel"); strings.Contains(rel, "nofollow") {
		return
	}

	href := c.getAttributeValue(linkNode, "href")
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return
	}

	linkText := c.extractTextContent(linkNode)
	if linkText != "" {
		pageData.LinkText = append(pageData.LinkText, linkText)
	}

	fullURL := c.constructFullURL(href, domain, protocol)
	if fullURL == "" {
		return
	}

	cleanURL, err := utils.CanonicalizeURL(fullURL)
	if err != nil {
		log.Printf("Failed to canonicalize URL %s: %v", fullURL, err)
		return
	}

	// Check if this is a PDF link
	if c.isPDFURL(cleanURL) && c.isAllowedOrigin(cleanURL) {
		log.Printf("Found PDF link: %s", cleanURL)

		// Process PDF asynchronously
		go func(pdfURL, pageURL string) {
			pdfData, err := c.processPDF(pdfURL, pageURL)
			if err != nil {
				log.Printf("Failed to process PDF %s: %v", pdfURL, err)
				return
			}

			// Add PDF data to page data
			c.Mu.Lock()
			pageData.PDFs = append(pageData.PDFs, *pdfData)
			c.Mu.Unlock()

			log.Printf("Successfully processed PDF with embeddings: %s", pdfURL)
		}(cleanURL, pageData.URL)

		// Don't enqueue PDFs in the crawl queue
		return
	}

	pageData.OutboundLinks = append(pageData.OutboundLinks, models.Link{
		Text: linkText,
		URL:  cleanURL,
	})

	// Only enqueue HTML links from allowed origins
	if c.isAllowedOrigin(cleanURL) {
		link := models.Link{Text: linkText, URL: cleanURL}
		c.safeEnqueue(link)
	}
}

func (c *Crawler) isMainContent(n *html.Node) bool {
	current := n.Parent
	for current != nil {
		if current.Type == html.ElementNode {
			switch current.Data {
			case "nav", "footer", "aside", "script", "style", "noscript", "header":
				return false
			}

			if id := c.getAttributeValue(current, "id"); id != "" {
				lowerID := strings.ToLower(id)
				skipPatterns := []string{"nav", "menu", "footer", "sidebar", "comment", "ad"}
				for _, pattern := range skipPatterns {
					if strings.Contains(lowerID, pattern) {
						return false
					}
				}
			}

			if class := c.getAttributeValue(current, "class"); class != "" {
				lowerClass := strings.ToLower(class)
				skipPatterns := []string{"nav", "menu", "footer", "sidebar", "comment", "ad"}
				for _, pattern := range skipPatterns {
					if strings.Contains(lowerClass, pattern) {
						return false
					}
				}
			}

			switch current.Data {
			case "main", "article":
				return true
			}
		}
		current = current.Parent
	}
	return true
}

func (c *Crawler) cleanContent(content string) string {
	content = strings.ToValidUTF8(content, "")
	content = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")

	scriptPatterns := []string{
		"JavaScript", "document.write", "function()", "var ", "const ", "let ",
		"window.", "document.", "addEventListener",
	}
	for _, pattern := range scriptPatterns {
		content = strings.ReplaceAll(content, pattern, "")
	}

	return strings.TrimSpace(content)
}

func (c *Crawler) getAttributeValue(n *html.Node, attrName string) string {
	for _, attr := range n.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}
	return ""
}

func (c *Crawler) extractTextContent(n *html.Node) string {
	var text strings.Builder
	c.extractTextOnly(n, &text)
	return strings.TrimSpace(text.String())
}

func (c *Crawler) extractTextOnly(n *html.Node, builder *strings.Builder) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if builder.Len() > 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(text)
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.extractTextOnly(child, builder)
	}
}

func (c *Crawler) constructFullURL(href, domain, protocol string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	} else if strings.HasPrefix(href, "//") {
		return "https:" + href
	} else if strings.HasPrefix(href, "/") {
		return protocol + domain + href
	} else {
		return protocol + domain + "/" + href
	}
}
