package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	healthy := isOllamaRunning()

	w.Header().Set("Content-Type", "application/json")
	status := "healthy"
	if !healthy {
		status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         status,
		"ollama_running": healthy,
		"ollama_url":     OLLAMA_URL,
		"model":          DEFAULT_MODEL,
	})
}

func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	ollamaReq := OllamaChatRequest{
		Model:    DEFAULT_MODEL,
		Messages: req.Messages,
		Stream:   true,
		Options:  make(map[string]interface{}),
	}

	if req.Temperature > 0 {
		ollamaReq.Options["temperature"] = req.Temperature
	} else {
		ollamaReq.Options["temperature"] = 0.7
	}

	if req.MaxTokens > 0 {
		ollamaReq.Options["num_predict"] = req.MaxTokens
	} else {
		ollamaReq.Options["num_predict"] = 200
	}

	ollamaReq.Options["num_ctx"] = 2048

	reqBody, _ := json.Marshal(ollamaReq)

	log.Printf("Sending to Ollama (streaming): %s", string(reqBody))

	startTime := time.Now()

	resp, err := http.Post(OLLAMA_URL+"/api/chat", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		log.Printf("Ollama error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Ollama request failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if req.Stream {
		streamToClient(w, resp.Body, startTime)
	} else {
		collectAndSend(w, resp.Body, startTime)
	}
}

func streamToClient(w http.ResponseWriter, ollamaBody io.Reader, startTime time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error": "Streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	decoder := json.NewDecoder(ollamaBody)
	responseID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	created := time.Now().Unix()
	totalContent := ""

	for {
		var ollamaResp OllamaChatResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Stream decode error: %v", err)
			break
		}

		totalContent += ollamaResp.Message.Content

		var finishReason *string
		if ollamaResp.Done {
			reason := "stop"
			finishReason = &reason
		}

		streamResp := StreamResponse{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   DEFAULT_MODEL,
			Choices: []StreamChoice{
				{
					Index: 0,
					Delta: ChatMessage{
						Role:    ollamaResp.Message.Role,
						Content: ollamaResp.Message.Content,
					},
					FinishReason: finishReason,
				},
			},
		}

		data, _ := json.Marshal(streamResp)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if ollamaResp.Done {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}
	}

	duration := time.Since(startTime)
	tokensGenerated := len(totalContent) / 4
	tokensPerSec := float64(tokensGenerated) / duration.Seconds()
	log.Printf("Streamed %d tokens in %v (%.1f tok/s)", tokensGenerated, duration, tokensPerSec)
}

func collectAndSend(w http.ResponseWriter, ollamaBody io.Reader, startTime time.Time) {
	decoder := json.NewDecoder(ollamaBody)
	var fullContent strings.Builder
	var lastRole string

	for {
		var ollamaResp OllamaChatResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Decode error: %v", err)
			http.Error(w, fmt.Sprintf(`{"error": "Failed to decode response: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		fullContent.WriteString(ollamaResp.Message.Content)
		lastRole = ollamaResp.Message.Role

		if ollamaResp.Done {
			break
		}
	}

	content := fullContent.String()
	if content == "" {
		log.Printf("Empty response from Ollama")
		http.Error(w, `{"error": "Empty response from Ollama"}`, http.StatusInternalServerError)
		return
	}

	duration := time.Since(startTime)
	tokensGenerated := len(content) / 4
	tokensPerSec := float64(tokensGenerated) / duration.Seconds()
	log.Printf("Generated %d tokens in %v (%.1f tok/s)", tokensGenerated, duration, tokensPerSec)

	response := ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   DEFAULT_MODEL,
		Choices: []ChatChoice{
			{
				Index:        0,
				Message:      ChatMessage{Role: lastRole, Content: content},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     0,
			CompletionTokens: tokensGenerated,
			TotalTokens:      tokensGenerated,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func completionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Prompt      string  `json:"prompt"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
		Temperature float64 `json:"temperature,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	chatReq := ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: req.Prompt},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	ollamaReq := OllamaChatRequest{
		Model:    DEFAULT_MODEL,
		Messages: chatReq.Messages,
		Stream:   false,
		Options:  make(map[string]interface{}),
	}

	if req.Temperature > 0 {
		ollamaReq.Options["temperature"] = req.Temperature
	} else {
		ollamaReq.Options["temperature"] = 0.7
	}

	if req.MaxTokens > 0 {
		ollamaReq.Options["num_predict"] = req.MaxTokens
	} else {
		ollamaReq.Options["num_predict"] = 256
	}

	reqBody, _ := json.Marshal(ollamaReq)

	startTime := time.Now()

	resp, err := http.Post(OLLAMA_URL+"/api/chat", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var ollamaResp OllamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	duration := time.Since(startTime)
	tokensGenerated := len(ollamaResp.Message.Content) / 4
	tokensPerSec := float64(tokensGenerated) / duration.Seconds()
	log.Printf("Generated %d tokens in %v (%.1f tok/s)", tokensGenerated, duration, tokensPerSec)

	response := map[string]interface{}{
		"id":      fmt.Sprintf("cmpl-%d", time.Now().Unix()),
		"object":  "text_completion",
		"created": time.Now().Unix(),
		"model":   DEFAULT_MODEL,
		"choices": []map[string]interface{}{
			{
				"text":          ollamaResp.Message.Content,
				"index":         0,
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": tokensGenerated,
			"total_tokens":      tokensGenerated,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
