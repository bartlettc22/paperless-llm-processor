# paperless-llm-processor

Automatically analyze, classify, and tag documents in [Paperless-ngx](https://docs.paperless-ngx.com/) using local vision LLMs via [Ollama](https://ollama.com/).

## What It Does

The batch processor fetches unprocessed documents from Paperless-ngx, converts them to images, sends each page to an Ollama vision model, and updates the documents with:

- **Title** - descriptive file name suggested by the LLM
- **Document type** - matched against your existing Paperless-ngx document types
- **Document date** - extracted from the document content
- **Summary** - concise summary stored in a custom field
- **Content** - full text transcription
- **Correspondent** - primary entity (person, business, organization)
- **Tags** - additional named entities mentioned in the document

Documents are tracked with an `llm-process-id` custom field, allowing re-processing by bumping the process ID. An `llm-skip` boolean custom field lets you exclude specific documents.

## Prerequisites

- [Ollama](https://ollama.com/) running with a vision model (e.g. `qwen3-vl:4b-instruct`, `qwen3-vl:8b-instruct`)
- [Paperless-ngx](https://docs.paperless-ngx.com/) instance with an API token
- `pdftoppm` from [poppler-utils](https://poppler.freedesktop.org/) installed on the system
- Go 1.25+

## Building

```bash
go build -o batch ./cmd/batch/
go build -o server ./cmd/server/
```

## Usage

### Batch Mode

Processes all unprocessed documents in Paperless-ngx:

```bash
export PAPERLESS_URL=http://localhost:8000
export PAPERLESS_TOKEN=your-api-token
export OLLAMA_URL=http://localhost:11434    # optional, this is the default
export OLLAMA_MODEL=qwen3-vl:4b-instruct   # optional, this is the default

./batch
```

#### Selective Field Updates

Use `UPDATE_FIELDS` to only update specific fields:

```bash
# Only update title and summary
UPDATE_FIELDS=title,summary ./batch

# Only update correspondent and tags
UPDATE_FIELDS=correspondent,tags ./batch
```

Valid fields: `title`, `document_type`, `document_date`, `summary`, `content`, `correspondent`, `tags`

### Server Mode

Runs an HTTP server for on-demand document analysis:

```bash
./server -ollama-url http://localhost:11434 -model qwen3-vl:4b-instruct -port 8080
```

Endpoints:

| Endpoint | Method | Description |
|---|---|---|
| `/analyze` | POST | Upload a document (multipart/form-data) for analysis |
| `/documents` | GET | List documents from Paperless-ngx |
| `/health` | GET | Health check |

## Custom Fields

The batch processor automatically creates these custom fields in Paperless-ngx:

| Field | Type | Description |
|---|---|---|
| `llm-process-id` | integer | Tracks which processing version last touched the document |
| `llm-summary` | longtext | AI-generated summary of the document |
| `llm-model` | string | The Ollama model that last processed the document |
| `llm-skip` | boolean | Set to true to exclude a document from processing |

## How Processing Works

1. Fetches documents where `llm-process-id` is null or less than the current process ID, excluding documents with `llm-skip` set to true
2. Downloads each document and converts to grayscale JPEG images (one per page)
3. Sends each page to the Ollama vision model for structured analysis
4. Merges results across pages (metadata from first page, summaries/transcriptions concatenated, tags deduplicated)
5. Creates correspondents and tags in Paperless-ngx if they don't exist
6. Updates the document with all extracted metadata
