package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

var ollamaCmd *exec.Cmd

func isOllamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

func installOllama() error {
	log.Println("Installing Ollama...")

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		log.Println("Downloading Ollama installer...")
		cmd = exec.Command("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	case "darwin":
		log.Println("Downloading Ollama for macOS...")
		cmd = exec.Command("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	case "windows":
		return fmt.Errorf("please install Ollama manually from https://ollama.com/download")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Ollama: %w", err)
	}

	log.Println("Ollama installed")
	return nil
}

func ensureOllamaInstalled() error {
	if isOllamaInstalled() {
		log.Println("Ollama is installed")
		return nil
	}

	log.Println("Ollama not found")
	return installOllama()
}

func startOllama() error {
	log.Println("Starting Ollama server...")

	if isOllamaRunning() {
		log.Println("Ollama already running")
		return nil
	}

	ollamaCmd = exec.Command("ollama", "serve")
	ollamaCmd.Stdout = os.Stdout
	ollamaCmd.Stderr = os.Stderr

	if err := ollamaCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}

	log.Printf("Ollama process started (PID: %d)", ollamaCmd.Process.Pid)

	log.Println("Waiting for Ollama to be ready...")
	for i := 0; i < 30; i++ {
		if isOllamaRunning() {
			log.Println("Ollama is ready")
			return nil
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("Ollama failed to start within 30 seconds")
}

func isOllamaRunning() bool {
	resp, err := http.Get(OLLAMA_URL + "/api/tags")
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return true
	}
	return false
}

func stopOllama() {
	if ollamaCmd != nil && ollamaCmd.Process != nil {
		log.Println("Stopping Ollama...")

		if err := ollamaCmd.Process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM: %v", err)
			ollamaCmd.Process.Kill()
		} else {
			done := make(chan error, 1)
			go func() {
				done <- ollamaCmd.Wait()
			}()

			select {
			case <-done:
				log.Println("Ollama stopped")
			case <-time.After(5 * time.Second):
				log.Println("Timeout waiting for Ollama, force killing...")
				ollamaCmd.Process.Kill()
			}
		}
	}
}

func ensureModelExists(model string) error {
	log.Printf("Checking if model '%s' exists...", model)

	resp, err := http.Get(OLLAMA_URL + "/api/tags")
	if err != nil {
		return fmt.Errorf("failed to check models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	for _, m := range result.Models {
		if len(m.Name) >= len(model) && m.Name[:len(model)] == model {
			log.Printf("Model '%s' found", model)
			return nil
		}
	}

	log.Printf("Pulling model '%s'...", model)
	log.Println("This may take a few minutes...")

	pullReq := map[string]string{"name": model}
	reqBody, _ := json.Marshal(pullReq)

	resp, err = http.Post(OLLAMA_URL+"/api/pull", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	lastStatus := ""
	lastProgress := 0

	for {
		var pullResp struct {
			Status    string `json:"status"`
			Completed int64  `json:"completed,omitempty"`
			Total     int64  `json:"total,omitempty"`
			Done      bool   `json:"done,omitempty"`
		}

		if err := decoder.Decode(&pullResp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading pull response: %w", err)
		}

		if pullResp.Total > 0 {
			progress := int((float64(pullResp.Completed) / float64(pullResp.Total)) * 100)
			if progress != lastProgress && progress%10 == 0 {
				log.Printf("Progress: %d%%", progress)
				lastProgress = progress
			}
		} else if pullResp.Status != lastStatus {
			log.Printf("%s", pullResp.Status)
			lastStatus = pullResp.Status
		}

		if pullResp.Done {
			break
		}
	}

	log.Printf("Model '%s' pulled successfully", model)
	return nil
}
