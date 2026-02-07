package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartlettc22/paperless-llm-processor/internal/converter"
	"github.com/bartlettc22/paperless-llm-processor/internal/ollama"
	"github.com/bartlettc22/paperless-llm-processor/internal/paperless"
)

func main() {
	paperlessURL := os.Getenv("PAPERLESS_URL")
	paperlessToken := os.Getenv("PAPERLESS_TOKEN")
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "qwen3-vl:8b"
	}

	if paperlessURL == "" || paperlessToken == "" {
		log.Fatal("PAPERLESS_URL and PAPERLESS_TOKEN must be set")
	}

	const processID = 1
	const fieldName = "llm-process-id"

	pClient := paperless.NewClient(paperlessURL, paperlessToken)
	oClient := ollama.NewClient(ollamaURL, ollamaModel)
	ctx := context.Background()

	cf, err := pClient.EnsureCustomField(ctx, fieldName, "integer")
	if err != nil {
		log.Fatalf("Failed to ensure custom field '%s': %v", fieldName, err)
	}
	log.Printf("Using custom field '%s' (id=%d), processID=%d", fieldName, cf.ID, processID)

	docs, err := pClient.ListUnprocessedDocuments(ctx, fieldName, processID)
	if err != nil {
		log.Fatalf("Failed to list unprocessed documents: %v", err)
	}

	log.Printf("Found %d unprocessed documents", len(docs))

	for _, doc := range docs {
		log.Printf("Processing document %d: %s", doc.ID, doc.Title)

		data, err := pClient.DownloadDocument(ctx, doc.ID)
		if err != nil {
			log.Printf("  ERROR downloading document %d: %v", doc.ID, err)
			continue
		}

		images, err := fileToBase64Images(data)
		if err != nil {
			log.Printf("  ERROR converting document %d: %v", doc.ID, err)
			continue
		}

		log.Printf("  Analyzing %d page(s) with %s...", len(images), ollamaModel)
		analysis, err := oClient.AnalyzeStructured(images)
		if err != nil {
			log.Printf("  ERROR analyzing document %d: %v", doc.ID, err)
			continue
		}

		result := map[string]interface{}{
			"document_id":    doc.ID,
			"document_title": doc.Title,
			"analysis":       analysis,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		fmt.Println()

		if err := pClient.UpdateDocumentCustomField(ctx, doc.ID, cf.ID, processID); err != nil {
			log.Printf("  ERROR updating custom field for document %d: %v", doc.ID, err)
			continue
		}
		log.Printf("  Marked document %d with %s=%d", doc.ID, fieldName, processID)
	}
}

// fileToBase64Images detects the file type and converts to base64-encoded images.
func fileToBase64Images(data []byte) ([]string, error) {
	contentType := http.DetectContentType(data)

	switch {
	case strings.HasPrefix(contentType, "application/pdf"):
		tmpFile, err := os.CreateTemp("", "doc-*.pdf")
		if err != nil {
			return nil, fmt.Errorf("creating temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("writing temp file: %w", err)
		}
		tmpFile.Close()
		return converter.PDFToBase64Images(tmpFile.Name(), "debug-images")

	case strings.HasPrefix(contentType, "image/"):
		tmpFile, err := os.CreateTemp("", "doc-*"+extForContentType(contentType))
		if err != nil {
			return nil, fmt.Errorf("creating temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("writing temp file: %w", err)
		}
		tmpFile.Close()
		img, err := converter.ImageToBase64(tmpFile.Name())
		if err != nil {
			return nil, err
		}
		return []string{img}, nil

	default:
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
}

func extForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg"):
		return ".jpg"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	default:
		return filepath.Ext(ct)
	}
}
