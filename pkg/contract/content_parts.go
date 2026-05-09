package contract

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	ContentPartTypeText      = "text"
	ContentPartTypeReasoning = "reasoning"
	ContentPartTypeImage     = "image"
	ContentPartTypeAudio     = "audio"
	ContentPartTypeFile      = "file"
)

func ValidateContentParts(parts []ContentPart) error {
	for i, part := range parts {
		if err := ValidateContentPart(part); err != nil {
			return fmt.Errorf("content part %d: %w", i, err)
		}
	}
	return nil
}

func ValidateContentPart(part ContentPart) error {
	partType := strings.TrimSpace(part.Type)
	if partType == "" {
		return fmt.Errorf("type is required")
	}
	switch partType {
	case ContentPartTypeText, ContentPartTypeReasoning:
		if strings.TrimSpace(part.Text) == "" {
			return fmt.Errorf("%s part requires text", partType)
		}
	case ContentPartTypeImage:
		if strings.TrimSpace(part.URL) == "" && strings.TrimSpace(part.Data) == "" {
			return fmt.Errorf("image part requires url or data")
		}
		if part.MIMEType != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.MIMEType)), "image/") {
			return fmt.Errorf("image part MIME type must start with image/")
		}
	case ContentPartTypeAudio:
		if strings.TrimSpace(part.URL) == "" && strings.TrimSpace(part.Data) == "" {
			return fmt.Errorf("audio part requires url or data")
		}
		if part.MIMEType != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.MIMEType)), "audio/") {
			return fmt.Errorf("audio part MIME type must start with audio/")
		}
	case ContentPartTypeFile:
		if strings.TrimSpace(part.URL) == "" && strings.TrimSpace(part.Data) == "" {
			return fmt.Errorf("file part requires url or data")
		}
	default:
		return fmt.Errorf("unsupported type %q", part.Type)
	}
	if rawURL := strings.TrimSpace(part.URL); rawURL != "" {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" {
			return fmt.Errorf("url must be absolute")
		}
		if parsed.Scheme != "data" && parsed.Host == "" {
			return fmt.Errorf("url must include a host")
		}
	}
	return nil
}
