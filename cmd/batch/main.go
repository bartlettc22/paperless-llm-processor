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
		// ollamaModel = "glm-ocr:latest"
		// ollamaModel = "glm-ocr:q8_0"
		// ollamaModel = "qwen3-vl:8b"
		// ollamaModel = "qwen3-vl:4b"
		// ollamaModel = "qwen3-vl:4b-instruct-q4_K_M"
		ollamaModel = "qwen3-vl:4b-instruct"
		// ollamaModel = "qwen3-vl:8b-instruct"
	}

	if paperlessURL == "" || paperlessToken == "" {
		log.Fatal("PAPERLESS_URL and PAPERLESS_TOKEN must be set")
	}

	// UPDATE_FIELDS controls which document fields to update (comma-separated).
	// Valid values: title, document_type, document_date, summary, content, correspondent, tags
	// If empty or unset, all fields are updated.
	updateFieldsEnv := os.Getenv("UPDATE_FIELDS")
	updateFields := map[string]bool{
		"title":         true,
		"document_type": true,
		"document_date": true,
		"summary":       true,
		"content":       true,
		"correspondent": true,
		"tags":          true,
	}
	if updateFieldsEnv != "" {
		updateFields = make(map[string]bool)
		for _, f := range strings.Split(updateFieldsEnv, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				updateFields[f] = true
			}
		}
		log.Printf("UPDATE_FIELDS: only updating %v", updateFieldsEnv)
	}

	const processID = 5
	const fieldName = "llm-process-id"

	pClient := paperless.NewClient(paperlessURL, paperlessToken)
	oClient := ollama.NewClient(ollamaURL, ollamaModel)
	ctx := context.Background()

	cf, err := pClient.EnsureCustomField(ctx, fieldName, "integer")
	if err != nil {
		log.Fatalf("Failed to ensure custom field '%s': %v", fieldName, err)
	}
	log.Printf("Using custom field '%s' (id=%d), processID=%d", fieldName, cf.ID, processID)

	const summaryFieldName = "llm-summary"
	summaryCF, err := pClient.EnsureCustomField(ctx, summaryFieldName, "longtext")
	if err != nil {
		log.Fatalf("Failed to ensure custom field '%s': %v", summaryFieldName, err)
	}
	log.Printf("Using custom field '%s' (id=%d)", summaryFieldName, summaryCF.ID)

	const modelFieldName = "llm-model"
	modelCF, err := pClient.EnsureCustomField(ctx, modelFieldName, "string")
	if err != nil {
		log.Fatalf("Failed to ensure custom field '%s': %v", modelFieldName, err)
	}
	log.Printf("Using custom field '%s' (id=%d)", modelFieldName, modelCF.ID)

	const skipFieldName = "llm-skip"
	_, err = pClient.EnsureCustomField(ctx, skipFieldName, "boolean")
	if err != nil {
		log.Fatalf("Failed to ensure custom field '%s': %v", skipFieldName, err)
	}
	log.Printf("Using custom field '%s' for skip filtering", skipFieldName)

	docTypes, err := pClient.ListDocumentTypes(ctx)
	if err != nil {
		log.Fatalf("Failed to list document types: %v", err)
	}
	docTypeNames := make([]string, len(docTypes))
	docTypeIDByName := make(map[string]int, len(docTypes))
	for i, dt := range docTypes {
		docTypeNames[i] = dt.Name
		docTypeIDByName[dt.Name] = dt.ID
	}
	log.Printf("Loaded %d document types: %v", len(docTypeNames), docTypeNames)

	// Load existing correspondents for lookup/creation
	corrList, err := pClient.ListCorrespondents(ctx)
	if err != nil {
		log.Fatalf("Failed to list correspondents: %v", err)
	}
	corrIDByName := make(map[string]int, len(corrList))
	for _, c := range corrList {
		corrIDByName[c.Name] = c.ID
	}
	log.Printf("Loaded %d correspondents", len(corrList))

	// Load existing tags for lookup/creation
	tagList, err := pClient.ListTags(ctx)
	if err != nil {
		log.Fatalf("Failed to list tags: %v", err)
	}
	tagIDByName := make(map[string]int, len(tagList))
	for _, t := range tagList {
		tagIDByName[t.Name] = t.ID
	}
	log.Printf("Loaded %d tags", len(tagList))

	docs, err := pClient.ListUnprocessedDocuments(ctx, fieldName, processID, skipFieldName)
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

		var merged ollama.DocumentAnalysis
		var summaries []string
		var transcriptions []string
		seenTags := make(map[string]bool)
		analyzeErr := false

		for i, img := range images {
			log.Printf("  Analyzing page %d/%d...", i+1, len(images))
			pageResult, err := oClient.AnalyzeStructured(img, docTypeNames)
			if err != nil {
				log.Printf("  ERROR analyzing document %d page %d: %v", doc.ID, i+1, err)
				analyzeErr = true
				break
			}

			if pageResult.Summary != "" {
				summaries = append(summaries, pageResult.Summary)
			}
			if pageResult.Transcription != "" {
				transcriptions = append(transcriptions, pageResult.Transcription)
			}

			// Use metadata from first page that provides it
			if merged.FileName == "" && pageResult.FileName != "" {
				merged.FileName = pageResult.FileName
			}
			if merged.DocumentType == "" && pageResult.DocumentType != "" {
				merged.DocumentType = pageResult.DocumentType
			}
			if merged.DocumentDate == "" && pageResult.DocumentDate != "" {
				merged.DocumentDate = pageResult.DocumentDate
			}
			if merged.Correspondent == "" && pageResult.Correspondent != "" {
				merged.Correspondent = pageResult.Correspondent
			}

			// Merge tags across pages (deduplicated)
			for _, t := range pageResult.Tags {
				if t != "" && !seenTags[t] {
					seenTags[t] = true
					merged.Tags = append(merged.Tags, t)
				}
			}

		}

		if analyzeErr {
			continue
		}

		merged.Summary = strings.Join(summaries, "\n\n")
		merged.Transcription = strings.Join(transcriptions, "\n\n")

		result := map[string]interface{}{
			"document_id":    doc.ID,
			"document_title": doc.Title,
			"analysis":       merged,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		fmt.Println()

		update := paperless.DocumentUpdate{
			CustomFields: []paperless.CustomFieldValue{
				{Field: cf.ID, Value: processID},
				{Field: modelCF.ID, Value: ollamaModel},
			},
		}

		if updateFields["title"] {
			update.Title = &merged.FileName
		}

		if updateFields["summary"] {
			update.CustomFields = append(update.CustomFields, paperless.CustomFieldValue{Field: summaryCF.ID, Value: merged.Summary})
		}

		if updateFields["content"] && merged.Transcription != "" {
			update.Content = &merged.Transcription
		}

		if updateFields["document_type"] {
			if dtID, ok := docTypeIDByName[merged.DocumentType]; ok {
				update.DocumentType = &dtID
			} else {
				log.Printf("  WARNING: unknown document type '%s', skipping type update", merged.DocumentType)
			}
		}

		if updateFields["document_date"] && merged.DocumentDate != "" {
			update.Created = &merged.DocumentDate
		}

		if updateFields["correspondent"] && merged.Correspondent != "" {
			corrID, err := pClient.EnsureCorrespondent(ctx, merged.Correspondent, corrIDByName)
			if err != nil {
				log.Printf("  WARNING: failed to ensure correspondent '%s': %v", merged.Correspondent, err)
			} else {
				update.Correspondent = &corrID
				log.Printf("  Correspondent: %s", merged.Correspondent)
			}
		}

		if updateFields["tags"] && len(merged.Tags) > 0 {
			var tagIDs []int
			for _, name := range merged.Tags {
				tagID, err := pClient.EnsureTag(ctx, name, tagIDByName)
				if err != nil {
					log.Printf("  WARNING: failed to ensure tag '%s': %v", name, err)
					continue
				}
				tagIDs = append(tagIDs, tagID)
			}
			if len(tagIDs) > 0 {
				update.Tags = tagIDs
				log.Printf("  Tags: %v", merged.Tags)
			}
		}

		if err := pClient.UpdateDocument(ctx, doc.ID, update); err != nil {
			log.Printf("  ERROR updating document %d: %v", doc.ID, err)
			continue
		}
		log.Printf("  Updated document %d: title=%s, type=%s, date=%s, %s=%d",
			doc.ID, merged.FileName, merged.DocumentType, merged.DocumentDate, fieldName, processID)
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
