package functions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	EmbeddingServiceURL = "http://localhost:5050/embed" // your embedding service
	EmbeddingTimeout    = 30 * time.Second              // timeout for calls
)

// request we send to FastAPI
type EmbedRequest struct {
	Text string `json:"text"`
}

// response we get back
type EmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Dims      int32     `json:"dims"`
	ElapsedMS float32   `json:"elapsed_ms"`
}

// client to talk to embedding service
type embeddingClient struct {
	httpClient *http.Client
	serviceURL string
}

var (
	defaultEmbeddingClient *embeddingClient // the one client we use everywhere
)

func init() {
	defaultEmbeddingClient = &embeddingClient{
		httpClient: &http.Client{
			Timeout: EmbeddingTimeout,
		},
		serviceURL: EmbeddingServiceURL,
	}
}

// GenerateEmbedding is the public func to get embeddings
func GenerateEmbedding(text string) ([]float32, error) {
	return defaultEmbeddingClient.generateEmbedding(text)
}

// internal call to actually do the HTTP request
func (ec *embeddingClient) generateEmbedding(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("text is empty, can't embed")
	}

	// optional: cut very long text
	const maxTextLength = 50000
	if len(text) > maxTextLength {
		log.Printf("text too long, cut from %d to %d chars", len(text), maxTextLength)
		text = text[:maxTextLength]
	}

	// build request
	reqBody := EmbedRequest{Text: text}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("can't marshal request: %w", err)
	}

	start := time.Now()
	resp, err := ec.httpClient.Post(ec.serviceURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("can't call embedding service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("service returned %d: %s", resp.StatusCode, string(body))
	}

	var embedResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("can't decode response: %w", err)
	}

	totalTime := time.Since(start).Milliseconds()
	log.Printf("Got embedding: %d dims, service: %.2fms, total: %dms",
		embedResp.Dims, embedResp.ElapsedMS, totalTime)

	return embedResp.Embedding, nil
}

// HealthCheck makes sure the embedding service is up
func HealthCheck() error {
	url := "http://localhost:5050/health"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("can't reach embedding service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service returned %d", resp.StatusCode)
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return fmt.Errorf("can't decode health response: %w", err)
	}

	log.Printf("Embedding service health: %v", health)
	return nil
}

// change service URL if needed
func SetEmbeddingServiceURL(url string) {
	defaultEmbeddingClient.serviceURL = url
	log.Printf("Service URL set to: %s", url)
}

// BatchGenerateEmbeddings for many texts at once
func BatchGenerateEmbeddings(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))

	for i, t := range texts {
		emb, err := GenerateEmbedding(t)
		if err != nil {
			return nil, fmt.Errorf("can't embed text %d: %w", i, err)
		}
		out[i] = emb

		// small pause so service doesn't freak out
		if i < len(texts)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	return out, nil
}

// GenerateEmbeddingWithRetry tries multiple times if it fails
func GenerateEmbeddingWithRetry(text string, maxRetries int) ([]float32, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		emb, err := GenerateEmbedding(text)
		if err == nil {
			return emb, nil
		}

		lastErr = err
		log.Printf("Try %d/%d failed: %v", attempt, maxRetries, err)

		if attempt < maxRetries {
			backoff := time.Duration(attempt*attempt) * time.Second
			log.Printf("Retry in %v...", backoff)
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("all %d tries failed: %w", maxRetries, lastErr)
}
