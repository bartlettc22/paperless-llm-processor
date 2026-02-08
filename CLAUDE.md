# CLAUDE.md

## Build & Run

```bash
# Build both binaries
go build -o batch ./cmd/batch/
go build -o server ./cmd/server/

# Run batch processing
PAPERLESS_URL=http://... PAPERLESS_TOKEN=... ./batch

# Run HTTP server
./server -ollama-url http://localhost:11434 -model qwen3-vl:4b-instruct -port 8080
```

## Project Structure

- `cmd/batch/` - Batch processor: fetches unprocessed docs from Paperless-ngx, analyzes via Ollama, updates documents
- `cmd/server/` - HTTP server with `/analyze`, `/documents`, `/health` endpoints
- `internal/ollama/` - Ollama vision API client (structured JSON output)
- `internal/paperless/` - Paperless-ngx REST API client (documents, custom fields, correspondents, tags)
- `internal/converter/` - PDF/image to base64 conversion (requires `pdftoppm` from poppler-utils)
- `internal/handler/` - HTTP handlers for server mode

## Key Environment Variables (Batch)

- `PAPERLESS_URL`, `PAPERLESS_TOKEN` (required)
- `OLLAMA_URL` (default: `http://localhost:11434`)
- `OLLAMA_MODEL` (default: `qwen3-vl:4b-instruct`)
- `UPDATE_FIELDS` - comma-separated list to selectively update: `title,document_type,document_date,summary,content,correspondent,tags`

## Custom Fields in Paperless-ngx

- `llm-process-id` (integer) - tracks processing version
- `llm-summary` (longtext) - AI-generated summary
- `llm-model` (string) - model that last processed the document
- `llm-skip` (boolean) - skip document from processing

## System Dependency

Requires `pdftoppm` (poppler-utils) for PDF conversion.
