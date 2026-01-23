package functions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"spider/db"
	"spider/models"
)

// check if URL is a PDF
func (c *Crawler) isPDFURL(urlStr string) bool {
	lowerURL := strings.ToLower(urlStr)
	return strings.HasSuffix(lowerURL, ".pdf") || strings.Contains(lowerURL, ".pdf?")
}

// download PDF, return local path
func (c *Crawler) downloadPDF(pdfURL, pageURL string) (string, error) {
	// skip if already processed
	c.Mu.Lock()
	if c.ProcessedPDFs[pdfURL] {
		c.Mu.Unlock()
		log.Printf("PDF already handled: %s", pdfURL)
		return "", fmt.Errorf("PDF already processed")
	}
	c.ProcessedPDFs[pdfURL] = true
	c.Mu.Unlock()

	// make unique filename
	hash := sha256.Sum256([]byte(pdfURL))
	hashStr := hex.EncodeToString(hash[:16])
	filename := hashStr + ".pdf"
	pdfPath := filepath.Join(c.pdfsDir, filename)

	// skip if file exists
	if _, err := os.Stat(pdfPath); err == nil {
		log.Printf("PDF exists: %s", pdfPath)
		return pdfPath, nil
	}

	// download with timeout
	ctx, cancel := context.WithTimeout(c.Ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", pdfURL, nil)
	if err != nil {
		return "", fmt.Errorf("can't make request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", pageURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't download PDF: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	// make sure it's really a PDF
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "pdf") {
		return "", fmt.Errorf("not a PDF type: %s", contentType)
	}

	// limit size 50MB
	limitedReader := io.LimitReader(resp.Body, 50*1024*1024)

	// write to temp file first
	tempPath := pdfPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("can't create file: %w", err)
	}

	_, err = io.Copy(file, limitedReader)
	file.Close()

	if err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("can't save PDF: %w", err)
	}

	// rename temp to final
	if err := os.Rename(tempPath, pdfPath); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("can't rename PDF: %w", err)
	}

	log.Printf("Downloaded PDF: %s -> %s", pdfURL, pdfPath)
	return pdfPath, nil
}

// extract text from PDF using pdftotext
func (c *Crawler) extractPDFText(pdfPath string) (string, error) {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("pdftotext missing, install poppler-utils: %w", err)
	}

	outputPath := pdfPath + ".txt"
	defer os.Remove(outputPath)

	cmd := exec.Command("pdftotext", "-layout", pdfPath, outputPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("can't extract text: %w", err)
	}

	textData, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("can't read text file: %w", err)
	}

	return string(textData), nil
}

// main PDF processing: download, extract text, generate embeddings
func (c *Crawler) processPDF(pdfURL, pageURL string) (*models.PDFData, error) {
	pdfPath, err := c.downloadPDF(pdfURL, pageURL)
	if err != nil {
		return nil, fmt.Errorf("can't download PDF: %w", err)
	}

	pdfText, err := c.extractPDFText(pdfPath)
	if err != nil {
		log.Printf("Warning: PDF downloaded but text extraction failed for %s: %v", pdfURL, err)
		pdfText = ""
	}

	pdfText = c.cleanContent(pdfText)

	fileInfo, err := os.Stat(pdfPath)
	var fileSize int64
	if err == nil {
		fileSize = fileInfo.Size()
	}

	pdfData := &models.PDFData{
		URL:      pdfURL,
		Path:     pdfPath,
		Text:     pdfText,
		FileSize: fileSize,
	}

	// process embeddings if we have text
	if pdfText != "" && c.embeddingFunc != nil {
		if err := c.processPDFEmbeddings(*pdfData, pageURL); err != nil {
			log.Printf("Warning: embedding failed for PDF %s: %v", pdfURL, err)
		}
	}

	log.Printf("Processed PDF: %s (size: %d bytes, text len: %d)", pdfURL, fileSize, len(pdfText))
	return pdfData, nil
}

// chunk PDF text and generate embeddings
func (c *Crawler) processPDFEmbeddings(pdf models.PDFData, pageURL string) error {
	if pdf.Text == "" {
		log.Printf("PDF has no text: %s, skip embedding", pdf.URL)
		return nil
	}

	chunks := ChunkText(pdf.Text, 500, 50)
	log.Printf("PDF %s split into %d chunks", pdf.URL, len(chunks))

	for i, chunk := range chunks {
		if len(strings.TrimSpace(chunk)) < 50 {
			continue
		}

		embedding, err := c.embeddingFunc(chunk)
		if err != nil {
			log.Printf("Can't embed chunk %d of PDF %s: %v", i, pdf.URL, err)
			continue
		}

		err = db.UpsertPDFEmbedding(pdf.URL, pdf.Path, pageURL, embedding, chunk, i)
		if err != nil {
			log.Printf("Can't store embedding for chunk %d of PDF %s: %v", i, pdf.URL, err)
			continue
		}

		log.Printf("Stored embedding for PDF %s chunk %d/%d", pdf.URL, i+1, len(chunks))
	}

	log.Printf("Done processing %d chunks for PDF %s", len(chunks), pdf.URL)
	return nil
}

// split text into chunks of given size with overlap
func ChunkText(text string, chunkSize, overlap int) []string {
	if len(text) == 0 {
		return []string{}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var chunks []string
	start := 0

	for start < len(words) {
		end := start + chunkSize
		if end > len(words) {
			end = len(words)
		}

		chunks = append(chunks, strings.Join(words[start:end], " "))

		if end == len(words) {
			break
		}

		start += (chunkSize - overlap)
	}

	return chunks
}
