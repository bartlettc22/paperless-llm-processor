package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartlettc22/paperless-llm-processor/internal/converter"
	"github.com/bartlettc22/paperless-llm-processor/internal/ollama"
)

type AnalyzeHandler struct {
	Client   *ollama.Client
	DebugDir string
}

type analyzeResponse struct {
	Filename string         `json:"filename"`
	Pages    []pageResponse `json:"pages"`
}

type pageResponse struct {
	Page     int    `json:"page"`
	Analysis string `json:"analysis"`
}

func (h *AnalyzeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100 MB max
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing 'file' field: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	prompt := r.FormValue("prompt")
	if prompt == "" {
		prompt = "Describe the contents of this document in detail."
	}

	tmpFile, err := os.CreateTemp("", "upload-*"+filepath.Ext(header.Filename))
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		http.Error(w, "failed to save uploaded file", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	var images []string

	switch ext {
	case ".pdf":
		images, err = converter.PDFToBase64Images(tmpFile.Name(), h.DebugDir)
		if err != nil {
			http.Error(w, "failed to convert PDF: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		img, err := converter.ImageToBase64(tmpFile.Name())
		if err != nil {
			http.Error(w, "failed to read image: "+err.Error(), http.StatusInternalServerError)
			return
		}
		images = []string{img}
	default:
		http.Error(w, fmt.Sprintf("unsupported file type: %s", ext), http.StatusBadRequest)
		return
	}

	resp := analyzeResponse{
		Filename: header.Filename,
		Pages:    make([]pageResponse, 0, len(images)),
	}

	for i, img := range images {
		pagePrompt := prompt
		if len(images) > 1 {
			pagePrompt = fmt.Sprintf("This is page %d of %d. %s", i+1, len(images), prompt)
		}

		log.Printf("Analyzing %s page %d/%d", header.Filename, i+1, len(images))
		analysis, err := h.Client.Analyze(pagePrompt, []string{img})
		if err != nil {
			http.Error(w, fmt.Sprintf("analysis failed on page %d: %s", i+1, err), http.StatusInternalServerError)
			return
		}

		resp.Pages = append(resp.Pages, pageResponse{
			Page:     i + 1,
			Analysis: analysis,
		})

		log.Printf("Completed %s page %d/%d", header.Filename, i+1, len(images))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
