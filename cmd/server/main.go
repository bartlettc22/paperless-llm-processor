package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bartlettc22/paperless-llm-processor/internal/handler"
	"github.com/bartlettc22/paperless-llm-processor/internal/ollama"
	"github.com/bartlettc22/paperless-llm-processor/internal/paperless"
)

func main() {
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Ollama API base URL")
	model := flag.String("model", "qwen3-vl:8b", "Ollama model to use for analysis")
	port := flag.Int("port", 8080, "HTTP server port")
	flag.Parse()

	client := ollama.NewClient(*ollamaURL, *model)

	var paperlessClient *paperless.Client
	paperlessURL := os.Getenv("PAPERLESS_URL")
	paperlessToken := os.Getenv("PAPERLESS_TOKEN")
	if paperlessURL != "" && paperlessToken != "" {
		paperlessClient = paperless.NewClient(paperlessURL, paperlessToken)
		log.Printf("Paperless-ngx configured at %s", paperlessURL)
	} else {
		log.Println("Paperless-ngx not configured (set PAPERLESS_URL and PAPERLESS_TOKEN)")
	}

	mux := http.NewServeMux()
	mux.Handle("/analyze", &handler.AnalyzeHandler{Client: client, DebugDir: "debug-images"})
	mux.Handle("/documents", &handler.DocumentsHandler{Client: paperlessClient})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s (ollama=%s, model=%s)", addr, *ollamaURL, *model)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
