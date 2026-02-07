package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

type generateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Images  []string        `json:"images"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options *modelOptions   `json:"options,omitempty"`
}

type modelOptions struct {
	Temperature float64 `json:"temperature"`
}

type DocumentAnalysis struct {
	Transcription string `json:"transcription"`
	FileName      string `json:"file_name"`
	DocumentType  string `json:"document_type"`
	DocumentDate  string `json:"document_date"`
}

type generateResponse struct {
	Response string `json:"response"`
	Thinking string `json:"thinking,omitempty"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

func NewClient(baseURL, model string) *Client {
	return &Client{
		BaseURL: baseURL,
		Model:   model,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// Analyze sends a base64-encoded image to the Ollama vision model with the given prompt.
func (c *Client) Analyze(prompt string, imagesBase64 []string) (string, error) {
	reqBody := generateRequest{
		Model:  c.Model,
		Prompt: prompt,
		Images: imagesBase64,
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.HTTP.Post(c.BaseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("calling ollama API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Response, nil
}

var documentAnalysisSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"transcription": {
			"type": "string",
			"description": "Full text transcription of the document"
		},
		"file_name": {
			"type": "string",
			"description": "Suggested file name for the document"
		},
		"document_type": {
			"type": "string",
			"enum": ["Invoice", "Receipt", "Letter", "Contract", "Tax Document", "Medical", "Insurance", "Bank Statement", "Other"],
			"description": "The type of document"
		},
		"document_date": {
			"type": "string",
			"description": "The date of the document in YYYY-MM-DD format, or empty string if not confidently determined"
		}
	},
	"required": ["transcription", "file_name", "document_type", "document_date"]
}`)

const structuredPrompt = `Analyze this document image and provide:
1. A complete transcription of all text in the document.
2. A suggested file name (descriptive, using underscores, with no extension).
3. The document type, which must be one of: Invoice, Receipt, Letter, Contract, Tax Document, Medical, Insurance, Bank Statement, Other.
4. The document date in YYYY-MM-DD format. Only provide a date if you are confident it is the primary date of the document (e.g. invoice date, letter date, transaction date). Use an empty string if uncertain.

Respond with JSON containing "transcription", "file_name", "document_type", and "document_date" fields.`

// AnalyzeStructured sends images to the Ollama vision model and returns structured analysis.
func (c *Client) AnalyzeStructured(imagesBase64 []string) (*DocumentAnalysis, error) {
	reqBody := generateRequest{
		Model:   c.Model,
		Prompt:  structuredPrompt,
		Images:  imagesBase64,
		Stream:  false,
		Format:  documentAnalysisSchema,
		Options: &modelOptions{Temperature: 0},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	log.Printf("  Sending request to Ollama (%d images, model=%s)...", len(imagesBase64), c.Model)

	resp, err := c.HTTP.Post(c.BaseURL+"/api/generate", "application/json", bytes.NewReader(body))
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

	var result generateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w: body=%s", err, string(respBody))
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	log.Printf("  Ollama response received (len=%d)", len(result.Response))

	// Some models (e.g. qwen3-vl) put structured output in the thinking field
	responseText := result.Response
	if responseText == "" {
		responseText = result.Thinking
	}

	if responseText == "" {
		return nil, fmt.Errorf("ollama returned empty response: full_body=%s", string(respBody))
	}

	var analysis DocumentAnalysis
	if err := json.Unmarshal([]byte(responseText), &analysis); err != nil {
		return nil, fmt.Errorf("parsing structured response: %w: raw=%s", err, responseText)
	}

	return &analysis, nil
}
