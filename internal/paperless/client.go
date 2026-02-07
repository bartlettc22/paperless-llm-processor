package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type Document struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type listResponse struct {
	Count   int        `json:"count"`
	Next    *string    `json:"next"`
	Results []Document `json:"results"`
}

type CustomField struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

type customFieldListResponse struct {
	Count   int           `json:"count"`
	Next    *string       `json:"next"`
	Results []CustomField `json:"results"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{},
	}
}

// ListCustomFields fetches all custom field definitions from Paperless-ngx.
func (c *Client) ListCustomFields(ctx context.Context) ([]CustomField, error) {
	var all []CustomField
	reqURL := c.BaseURL + "/api/custom_fields/"

	for reqURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching custom fields: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
		}

		var page customFieldListResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		all = append(all, page.Results...)

		if page.Next != nil {
			reqURL = *page.Next
		} else {
			reqURL = ""
		}
	}

	return all, nil
}

// CreateCustomField creates a new custom field definition in Paperless-ngx.
func (c *Client) CreateCustomField(ctx context.Context, name, dataType string) (CustomField, error) {
	body, _ := json.Marshal(map[string]string{"name": name, "data_type": dataType})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/custom_fields/", bytes.NewReader(body))
	if err != nil {
		return CustomField{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return CustomField{}, fmt.Errorf("creating custom field: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return CustomField{}, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var cf CustomField
	if err := json.NewDecoder(resp.Body).Decode(&cf); err != nil {
		return CustomField{}, fmt.Errorf("decoding response: %w", err)
	}
	return cf, nil
}

// EnsureCustomField returns the custom field with the given name, creating it if it doesn't exist.
func (c *Client) EnsureCustomField(ctx context.Context, name, dataType string) (CustomField, error) {
	fields, err := c.ListCustomFields(ctx)
	if err != nil {
		return CustomField{}, fmt.Errorf("listing custom fields: %w", err)
	}
	for _, f := range fields {
		if f.Name == name {
			return f, nil
		}
	}
	return c.CreateCustomField(ctx, name, dataType)
}

// UpdateDocumentCustomField sets a custom field value on a document.
func (c *Client) UpdateDocumentCustomField(ctx context.Context, documentID, fieldID, value int) error {
	type cfValue struct {
		Field int `json:"field"`
		Value int `json:"value"`
	}
	payload := map[string][]cfValue{
		"custom_fields": {{Field: fieldID, Value: value}},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("%s/api/documents/%d/", c.BaseURL, documentID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("updating document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DownloadDocument downloads the original file for a document by ID.
func (c *Client) DownloadDocument(ctx context.Context, documentID int) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/api/documents/%d/download/", c.BaseURL, documentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// ListUnprocessedDocuments fetches documents where the custom field is null or less than processID.
// Makes two queries and deduplicates by document ID.
func (c *Client) ListUnprocessedDocuments(ctx context.Context, fieldName string, processID int) ([]Document, error) {
	// Query 1: field is null (not set on the document)
	nullQuery, _ := json.Marshal([]interface{}{fieldName, "isnull", true})
	nullDocs, err := c.listDocuments(ctx, fmt.Sprintf("&custom_field_query=%s", url.QueryEscape(string(nullQuery))))
	if err != nil {
		return nil, fmt.Errorf("querying null documents: %w", err)
	}

	// Query 2: field value < processID
	ltQuery, _ := json.Marshal([]interface{}{fieldName, "lt", processID})
	ltDocs, err := c.listDocuments(ctx, fmt.Sprintf("&custom_field_query=%s", url.QueryEscape(string(ltQuery))))
	if err != nil {
		return nil, fmt.Errorf("querying lt documents: %w", err)
	}

	// Deduplicate
	seen := make(map[int]bool)
	var result []Document
	for _, doc := range append(nullDocs, ltDocs...) {
		if !seen[doc.ID] {
			seen[doc.ID] = true
			result = append(result, doc)
		}
	}

	return result, nil
}

// ListDocuments fetches all documents from Paperless-ngx, handling pagination.
func (c *Client) ListDocuments(ctx context.Context) ([]Document, error) {
	return c.listDocuments(ctx, "")
}

func (c *Client) listDocuments(ctx context.Context, extraQuery string) ([]Document, error) {
	var all []Document
	reqURL := c.BaseURL + "/api/documents/?fields=id,title" + extraQuery

	for reqURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching documents: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
		}

		var page listResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		all = append(all, page.Results...)

		if page.Next != nil {
			reqURL = *page.Next
		} else {
			reqURL = ""
		}
	}

	return all, nil
}
