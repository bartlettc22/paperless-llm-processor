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

type DocumentType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type documentTypeListResponse struct {
	Count   int            `json:"count"`
	Next    *string        `json:"next"`
	Results []DocumentType `json:"results"`
}

// ListDocumentTypes fetches all document types from Paperless-ngx.
func (c *Client) ListDocumentTypes(ctx context.Context) ([]DocumentType, error) {
	var all []DocumentType
	reqURL := c.BaseURL + "/api/document_types/?fields=id,name"

	for reqURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching document types: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
		}

		var page documentTypeListResponse
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

type Correspondent struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type correspondentListResponse struct {
	Count   int             `json:"count"`
	Next    *string         `json:"next"`
	Results []Correspondent `json:"results"`
}

// ListCorrespondents fetches all correspondents from Paperless-ngx.
func (c *Client) ListCorrespondents(ctx context.Context) ([]Correspondent, error) {
	var all []Correspondent
	reqURL := c.BaseURL + "/api/correspondents/?fields=id,name"

	for reqURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching correspondents: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
		}

		var page correspondentListResponse
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

// CreateCorrespondent creates a new correspondent in Paperless-ngx.
func (c *Client) CreateCorrespondent(ctx context.Context, name string) (Correspondent, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/correspondents/", bytes.NewReader(body))
	if err != nil {
		return Correspondent{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Correspondent{}, fmt.Errorf("creating correspondent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return Correspondent{}, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var corr Correspondent
	if err := json.NewDecoder(resp.Body).Decode(&corr); err != nil {
		return Correspondent{}, fmt.Errorf("decoding response: %w", err)
	}
	return corr, nil
}

// EnsureCorrespondent returns the correspondent with the given name, creating it if it doesn't exist.
func (c *Client) EnsureCorrespondent(ctx context.Context, name string, existing map[string]int) (int, error) {
	if id, ok := existing[name]; ok {
		return id, nil
	}
	corr, err := c.CreateCorrespondent(ctx, name)
	if err != nil {
		return 0, err
	}
	existing[name] = corr.ID
	return corr.ID, nil
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tagListResponse struct {
	Count   int     `json:"count"`
	Next    *string `json:"next"`
	Results []Tag   `json:"results"`
}

// ListTags fetches all tags from Paperless-ngx.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var all []Tag
	reqURL := c.BaseURL + "/api/tags/?fields=id,name"

	for reqURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Token "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching tags: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(body))
		}

		var page tagListResponse
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

// CreateTag creates a new tag in Paperless-ngx.
func (c *Client) CreateTag(ctx context.Context, name string) (Tag, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/tags/", bytes.NewReader(body))
	if err != nil {
		return Tag{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Tag{}, fmt.Errorf("creating tag: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return Tag{}, fmt.Errorf("paperless returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tag Tag
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		return Tag{}, fmt.Errorf("decoding response: %w", err)
	}
	return tag, nil
}

// EnsureTag returns the tag ID for the given name, creating it if it doesn't exist.
func (c *Client) EnsureTag(ctx context.Context, name string, existing map[string]int) (int, error) {
	if id, ok := existing[name]; ok {
		return id, nil
	}
	tag, err := c.CreateTag(ctx, name)
	if err != nil {
		return 0, err
	}
	existing[name] = tag.ID
	return tag.ID, nil
}

// CustomFieldValue represents a custom field value to set on a document.
type CustomFieldValue struct {
	Field int         `json:"field"`
	Value interface{} `json:"value"`
}

// DocumentUpdate holds the fields to update on a document via PATCH.
type DocumentUpdate struct {
	Title         *string            `json:"title,omitempty"`
	Content       *string            `json:"content,omitempty"`
	DocumentType  *int               `json:"document_type,omitempty"`
	Correspondent *int               `json:"correspondent,omitempty"`
	Tags          []int              `json:"tags,omitempty"`
	Created       *string            `json:"created,omitempty"`
	CustomFields  []CustomFieldValue `json:"custom_fields,omitempty"`
}

// UpdateDocument patches a document with the provided fields.
func (c *Client) UpdateDocument(ctx context.Context, documentID int, update DocumentUpdate) error {
	body, _ := json.Marshal(update)

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

// ListUnprocessedDocuments fetches documents where the custom field is null or less than processID,
// excluding any documents where skipFieldName is set to true.
func (c *Client) ListUnprocessedDocuments(ctx context.Context, fieldName string, processID int, skipFieldName string) ([]Document, error) {
	// Query 1: field does not exist on the document
	existsQuery, _ := json.Marshal([]interface{}{fieldName, "exists", false})
	nullDocs, err := c.listDocuments(ctx, fmt.Sprintf("&custom_field_query=%s", url.QueryEscape(string(existsQuery))))
	if err != nil {
		return nil, fmt.Errorf("querying null documents: %w", err)
	}

	// Query 2: field value < processID
	ltQuery, _ := json.Marshal([]interface{}{fieldName, "lt", processID})
	ltDocs, err := c.listDocuments(ctx, fmt.Sprintf("&custom_field_query=%s", url.QueryEscape(string(ltQuery))))
	if err != nil {
		return nil, fmt.Errorf("querying lt documents: %w", err)
	}

	// Find documents to skip (llm-skip == true)
	skipIDs := make(map[int]bool)
	if skipFieldName != "" {
		skipQuery, _ := json.Marshal([]interface{}{skipFieldName, "exact", true})
		skipDocs, err := c.listDocuments(ctx, fmt.Sprintf("&custom_field_query=%s", url.QueryEscape(string(skipQuery))))
		if err != nil {
			return nil, fmt.Errorf("querying skip documents: %w", err)
		}
		for _, doc := range skipDocs {
			skipIDs[doc.ID] = true
		}
	}

	// Deduplicate and exclude skipped
	seen := make(map[int]bool)
	var result []Document
	for _, doc := range append(nullDocs, ltDocs...) {
		if !seen[doc.ID] && !skipIDs[doc.ID] {
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
