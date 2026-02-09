package ollama

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

type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
	Think    bool            `json:"think"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  *modelOptions   `json:"options,omitempty"`
}

type chatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type modelOptions struct {
	Temperature   float64 `json:"temperature"`
	RepeatPenalty float64 `json:"repeat_penalty,omitempty"`
	NumPredict    int     `json:"num_predict,omitempty"`
	NumCtx        int     `json:"num_ctx,omitempty"`
}

type DocumentAnalysis struct {
	Summary       string   `json:"summary"`
	Transcription string   `json:"transcription"`
	FileName      string   `json:"file_name"`
	DocumentType  string   `json:"document_type"`
	DocumentDate  string   `json:"document_date"`
	Correspondent string   `json:"correspondent"`
	Tags          []string `json:"tags"`
}

type chatResponse struct {
	Message    chatResponseMessage `json:"message"`
	Done       bool                `json:"done"`
	DoneReason string              `json:"done_reason,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type chatResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func NewClient(baseURL, model string) *Client {
	return &Client{
		BaseURL: baseURL,
		Model:   model,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// Analyze sends base64-encoded images to the Ollama vision model with the given prompt.
func (c *Client) Analyze(prompt string, imagesBase64 []string) (string, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt, Images: imagesBase64},
		},
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.HTTP.Post(c.BaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("calling ollama API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Message.Content, nil
}

func buildSchema(documentTypes []string) json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_name": map[string]interface{}{
				"type":        "string",
				"description": "Suggested file name for the document",
			},
			"document_type": map[string]interface{}{
				"type":        "string",
				"enum":        documentTypes,
				"description": "The type of document",
			},
			"document_date": map[string]interface{}{
				"type":        "string",
				"description": "The date of the document in YYYY-MM-DD format, or empty string if not confidently determined",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "A concise summary of the document including what it is, relevant dates, people, transactions, entities, accounts, and key details.",
			},
			"transcription": map[string]interface{}{
				"type":        "string",
				"description": "A full transcription of all visible text on this page, preserving the original wording and layout as much as possible.",
			},
			"correspondent": map[string]interface{}{
				"type":        "string",
				"description": "The primary correspondent: the person, business, organization, or entity that sent or is the main subject of this document. Use proper name and title case. Empty string if none.",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"description": "ONLY proper names of specific people, companies, or organizations (e.g. 'John Smith', 'Acme Corp', 'IRS'). NEVER include generic terms, descriptions, diagnoses, topics, or categories. If no proper names apply, return an empty array.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"summary", "transcription", "file_name", "document_type", "document_date", "correspondent", "tags"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func buildPrompt(documentTypes []string) string {
	typeList := strings.Join(documentTypes, ", ")
	return fmt.Sprintf(`You are looking at a single page of a document. Analyze this page image and provide:
1. A concise summary of this page's content: what it is, relevant dates, people, transactions, entities, accounts, and any other key details.
2. A full transcription of all visible text on this page. Preserve the meaningful content and general structure, but normalize whitespace - use single spaces between words and single newlines between lines or sections. Do NOT repeat tabs, newlines, or spaces excessively. For barcodes, tracking numbers, or long sequences of repeated characters, just note their presence (e.g. "[barcode]") rather than transcribing every digit.
3. A suggested file name (descriptive, using underscores, with no extension).
4. The document type, which must be one of: %s.
5. The document date in YYYY-MM-DD format. Only provide a date if you are confident it is the primary date of the document (e.g. invoice date, letter date, transaction date). Use an empty string if uncertain.
6. The correspondent: the primary person, business, organization, or entity that sent or is the main subject of this document. Use proper name and title case. Use an empty string if none.
7. Tags: ONLY proper names of specific people, companies, or organizations mentioned in the document (e.g. "John Smith", "Acme Corp", "IRS"). NEVER include generic terms, descriptions, diagnoses, topics, or categories (e.g. do NOT include things like "Left lower quadrant pain", "Invoice", "Medical Records"). If no proper names apply, return an empty array.

Respond with JSON containing "summary", "transcription", "file_name", "document_type", "document_date", "correspondent", and "tags" fields.  The response MUST be valid JSON.`, typeList)
}

// AnalyzeStructured sends a single page image to the Ollama vision model and returns structured analysis.
// documentTypes is the list of valid document type names from Paperless-ngx.
func (c *Client) AnalyzeStructured(imageBase64 string, documentTypes []string) (*DocumentAnalysis, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "user", Content: buildPrompt(documentTypes), Images: []string{imageBase64}},
		},
		Stream:  false,
		Think:   false,
		Format:  buildSchema(documentTypes),
		Options: &modelOptions{
			Temperature:   0,
			NumCtx:        65536, // Use more of the 128k context
			NumPredict:    16384, // Allow very long transcriptions
			RepeatPenalty: 1.5,   // Discourage repetitive patterns (whitespace, barcodes, zeros)
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	log.Printf("  Sending request to Ollama (model=%s, num_ctx=%d, num_predict=%d)...",
		c.Model, reqBody.Options.NumCtx, reqBody.Options.NumPredict)

	resp, err := c.HTTP.Post(c.BaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("calling ollama API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w: body=%s", err, string(respBody))
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	content := result.Message.Content
	log.Printf("  Ollama response: done=%v, done_reason=%q, content_len=%d", result.Done, result.DoneReason, len(content))

	if !result.Done {
		log.Printf("  WARNING: Ollama returned incomplete response (done=false, reason=%q)", result.DoneReason)
	}

	if content == "" {
		return nil, fmt.Errorf("ollama returned empty response: full_body=%s", string(respBody))
	}

	log.Printf("  Response (first_200=%s ... last_100=%s)", truncateHead(content, 200), truncateTail(content, 100))

	var analysis DocumentAnalysis
	if err := json.Unmarshal([]byte(content), &analysis); err != nil {
		return nil, fmt.Errorf("parsing response: %w: len=%d, done=%v, last_200=%s", err, len(content), result.Done, truncateTail(content, 200))
	}

	return &analysis, nil
}

func truncateTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func truncateHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
