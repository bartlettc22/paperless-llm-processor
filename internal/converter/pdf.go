package converter

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// PDFToBase64Images converts a PDF file to a slice of base64-encoded JPEG images,
// one per page. Uses grayscale, JPEG quality 80, and scales to max 1568px.
// Requires pdftoppm (poppler-utils) to be installed.
// Images are also saved to debugDir for inspection.
func PDFToBase64Images(pdfPath, debugDir string) ([]string, error) {
	tmpDir, err := os.MkdirTemp("", "pdf-convert-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPrefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command("pdftoppm",
		"-jpeg", "-jpegopt", "quality=80",
		"-gray",
		"-scale-to", "768",
		pdfPath, outputPrefix,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("running pdftoppm: %w: %s", err, string(output))
	}

	matches, err := filepath.Glob(outputPrefix + "-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("globbing output files: %w", err)
	}
	sort.Strings(matches)

	if debugDir != "" {
		if err := os.MkdirAll(debugDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating debug dir: %w", err)
		}
	}

	images := make([]string, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		images = append(images, base64.StdEncoding.EncodeToString(data))

		if debugDir != "" {
			debugPath := filepath.Join(debugDir, filepath.Base(path))
			if err := os.WriteFile(debugPath, data, 0o644); err != nil {
				return nil, fmt.Errorf("writing debug image %s: %w", debugPath, err)
			}
		}
	}

	return images, nil
}

// ImageToBase64 reads an image file and returns its base64-encoded content.
func ImageToBase64(imagePath string) (string, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("reading image %s: %w", imagePath, err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
