package functions

import (
	"fmt"
	"log"
	"spider/db"
)

// SearchPDFContent does semantic search over all PDFs
func SearchPDFContent(query string, limit uint64, embeddingFunc func(string) ([]float32, error)) ([]map[string]interface{}, error) {
	// make embedding for the query
	queryEmbedding, err := embeddingFunc(query)
	if err != nil {
		return nil, fmt.Errorf("can't make query embedding: %w", err)
	}

	// search in Qdrant
	results, err := db.SearchPDFEmbeddings(queryEmbedding, limit)
	if err != nil {
		return nil, fmt.Errorf("PDF search failed: %w", err)
	}

	// format results
	formatted := make([]map[string]interface{}, len(results))
	for i, res := range results {
		payload := res.GetPayload()
		formatted[i] = map[string]interface{}{
			"pdf_url":     payload["pdf_url"],
			"pdf_path":    payload["pdf_path"],
			"page_url":    payload["page_url"],
			"text_chunk":  payload["text_chunk"],
			"chunk_index": payload["chunk_index"],
			"score":       res.GetScore(),
		}
	}

	return formatted, nil
}

// HybridContentSearch searches both pages and PDFs
func HybridContentSearch(query string, limit uint64, embeddingFunc func(string) ([]float32, error)) (map[string]interface{}, error) {
	// make embedding
	queryEmbedding, err := embeddingFunc(query)
	if err != nil {
		return nil, fmt.Errorf("can't make query embedding: %w", err)
	}

	results, err := db.HybridSearch(queryEmbedding, limit)
	if err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}

	// format results
	out := map[string]interface{}{
		"pages": make([]map[string]interface{}, 0),
		"pdfs":  make([]map[string]interface{}, 0),
	}

	if pageResults, ok := results["pages"]; ok {
		for _, res := range pageResults {
			payload := res.GetPayload()
			out["pages"] = append(out["pages"].([]map[string]interface{}), map[string]interface{}{
				"url":   payload["url"],
				"text":  payload["text"],
				"score": res.GetScore(),
			})
		}
	}

	if pdfResults, ok := results["pdfs"]; ok {
		for _, res := range pdfResults {
			payload := res.GetPayload()
			out["pdfs"] = append(out["pdfs"].([]map[string]interface{}), map[string]interface{}{
				"pdf_url":     payload["pdf_url"],
				"pdf_path":    payload["pdf_path"],
				"page_url":    payload["page_url"],
				"text_chunk":  payload["text_chunk"],
				"chunk_index": payload["chunk_index"],
				"score":       res.GetScore(),
			})
		}
	}

	return out, nil
}

// GetPDFsByPageURL gets all PDFs found on a specific page
func GetPDFsByPageURL(pageURL string, limit uint64) ([]map[string]interface{}, error) {
	results, err := db.SearchPDFsByPageURL(pageURL, limit)
	if err != nil {
		return nil, fmt.Errorf("can't get PDFs by page URL: %w", err)
	}

	out := make([]map[string]interface{}, len(results))
	for i, res := range results {
		payload := res.GetPayload()
		out[i] = map[string]interface{}{
			"pdf_url":     payload["pdf_url"],
			"pdf_path":    payload["pdf_path"],
			"text_chunk":  payload["text_chunk"],
			"chunk_index": payload["chunk_index"],
		}
	}

	return out, nil
}

// DeletePDFByURL removes all embeddings for a PDF
func DeletePDFByURL(pdfURL string) error {
	if err := db.DeletePDFEmbeddings(pdfURL); err != nil {
		return fmt.Errorf("can't delete PDF embeddings: %w", err)
	}
	log.Printf("Deleted all embeddings for PDF: %s", pdfURL)
	return nil
}

// GetPDFStats returns stats about PDF embeddings
func GetPDFStats() (map[string]interface{}, error) {
	stats, err := db.GetPDFEmbeddingsStats()
	if err != nil {
		return nil, fmt.Errorf("can't get PDF stats: %w", err)
	}
	return stats, nil
}

// i moved the search into the server service anyways
