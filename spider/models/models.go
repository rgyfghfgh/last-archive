package models

import "time"

// Link represents a hyperlink found on a page
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Image represents an image found on a page
type Image struct {
	URL  string `json:"url"`
	Alt  string `json:"alt"`
	Path string `json:"path"` // Local filesystem path where image is stored
}

// PDFData represents a PDF file found and downloaded
type PDFData struct {
	URL      string `json:"url"`       // Original URL of the PDF
	Path     string `json:"path"`      // Local filesystem path where PDF is stored
	Text     string `json:"text"`      // Extracted text content from PDF
	FileSize int64  `json:"file_size"` // Size of PDF file in bytes
}

// PageData represents all data extracted from a crawled page
type PageData struct {
	URL             string              `json:"url"`
	Title           string              `json:"title"`
	MetaDescription string              `json:"meta_description"`
	MetaKeywords    string              `json:"meta_keywords"`
	Canonical       string              `json:"canonical"`
	Language        string              `json:"language"`
	Favicon         string              `json:"favicon"`
	Headings        map[string][]string `json:"headings"` // h1, h2, h3, etc.
	MainContent     string              `json:"main_content"`
	ImageAlt        []string            `json:"image_alt"`
	Images          []Image             `json:"images"` // Image URLs with local paths
	PDFs            []PDFData           `json:"pdfs"`   // PDF files found on the page
	LinkText        []string            `json:"link_text"`
	OutboundLinks   []Link              `json:"outbound_links"`
	WordCount       int                 `json:"word_count"`
	StatusCode      int                 `json:"status_code"`
	ResponseTime    time.Duration       `json:"response_time"`
	ContentType     string              `json:"content_type"`
	CrawlDate       time.Time           `json:"crawl_date"`
	LastModified    time.Time           `json:"last_modified"`
}

type EmbeddingModel struct {
	Embedding  []float32 `json:"embedding"`
	Dims       int32     `json:"dims"`
	ELAPSED_MS float32   `json:"elapsed_ms"`
}
