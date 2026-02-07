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
	Summary        string   `json:"summary"`
	FileName       string   `json:"file_name"`
	DocumentType   string   `json:"document_type"`
	DocumentDate   string   `json:"document_date"`
	Correspondents []string `json:"correspondents"`
}

type chatResponse struct {
	Message chatResponseMessage `json:"message"`
	Done    bool                `json:"done"`
	Error   string              `json:"error,omitempty"`
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
			"correspondents": map[string]interface{}{
				"type":        "array",
				"description": "A list of entities (people, businesses, organizations, government agencies, etc.) that this document pertains to.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"summary", "file_name", "document_type", "document_date", "correspondents"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func buildPrompt(documentTypes []string) string {
	typeList := strings.Join(documentTypes, ", ")
	return fmt.Sprintf(`You are looking at a single page of a document. Analyze this page image and provide:
1. A concise summary of this page's content: what it is, relevant dates, people, transactions, entities, accounts, and any other key details.
2. A suggested file name (descriptive, using underscores, with no extension).
3. The document type, which must be one of: %s.
4. The document date in YYYY-MM-DD format. Only provide a date if you are confident it is the primary date of the document (e.g. invoice date, letter date, transaction date). Use an empty string if uncertain.
5. A list of correspondents: the people, businesses, organizations, government agencies, or other entities that this document pertains to. Use proper names and title case.

Respond with JSON containing "summary", "file_name", "document_type", "document_date", and "correspondents" fields.  The response MUST be valid JSON.`, typeList)
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
		Options: &modelOptions{Temperature: 0},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	log.Printf("  Sending request to Ollama (model=%s)...", c.Model)

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
	log.Printf("  Ollama response: done=%v, content_len=%d", result.Done, len(content))

	if content == "" {
		return nil, fmt.Errorf("ollama returned empty response: full_body=%s", string(respBody))
	}

	log.Printf("  Response (last_100=%s)", truncateTail(content, 100))

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
