package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

const (
	OLLAMA_URL    = "http://localhost:11434"
	DEFAULT_MODEL = "qwen:0.5b"
)

func setupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		stopOllama()
		os.Exit(0)
	}()
}

func main() {
	log.Println("========================================")
	log.Println("LLM Server with Ollama Backend")
	log.Println("========================================")

	setupSignalHandler()

	log.Println("Step 1: Checking Ollama installation...")
	if err := ensureOllamaInstalled(); err != nil {
		log.Fatalf("Failed to ensure Ollama is installed: %v\nInstall manually: curl -fsSL https://ollama.com/install.sh | sh", err)
	}

	log.Println("Step 2: Starting Ollama server...")
	if err := startOllama(); err != nil {
		log.Fatalf("Failed to start Ollama: %v", err)
	}

	log.Println("Step 3: Ensuring model is available...")
	if err := ensureModelExists(DEFAULT_MODEL); err != nil {
		log.Fatalf("Failed to ensure model exists: %v", err)
	}

	log.Println("Step 4: Setting up HTTP server...")
	http.HandleFunc("/health", corsMiddleware(healthHandler))
	http.HandleFunc("/v1/chat/completions", corsMiddleware(chatCompletionsHandler))
	http.HandleFunc("/v1/completions", corsMiddleware(completionsHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "1410"
	}

	addr := fmt.Sprintf("0.0.0.0:%s", port)

	log.Println("========================================")
	log.Println("Server is ready")
	log.Println("========================================")
	log.Printf("URL: http://%s", addr)
	log.Println("Endpoints:")
	log.Println("   GET  /health")
	log.Println("   POST /v1/chat/completions")
	log.Println("   POST /v1/completions")
	log.Printf("Backend: Ollama at %s", OLLAMA_URL)
	log.Printf("Model: %s", DEFAULT_MODEL)
	log.Println("========================================")

	if err := http.ListenAndServe(addr, nil); err != nil {
		stopOllama()
		log.Fatalf("Server failed: %v", err)
	}
}
