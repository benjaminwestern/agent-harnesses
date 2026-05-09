package contract

import "testing"

func TestValidateContentPartsAcceptsKnownShapes(t *testing.T) {
	parts := []ContentPart{
		{Type: ContentPartTypeText, Text: "hello"},
		{Type: ContentPartTypeReasoning, Text: "thinking"},
		{Type: ContentPartTypeImage, MIMEType: "image/png", Data: "aW1hZ2U="},
		{Type: ContentPartTypeAudio, MIMEType: "audio/wav", URL: "https://example.com/audio.wav"},
		{Type: ContentPartTypeFile, URL: "https://example.com/file.pdf"},
	}
	if err := ValidateContentParts(parts); err != nil {
		t.Fatalf("ValidateContentParts failed: %v", err)
	}
}

func TestValidateContentPartsRejectsUnsupportedAndIncompleteParts(t *testing.T) {
	tests := []struct {
		name string
		part ContentPart
	}{
		{name: "missing type", part: ContentPart{Text: "hello"}},
		{name: "empty image", part: ContentPart{Type: ContentPartTypeImage}},
		{name: "unsupported", part: ContentPart{Type: "video", URL: "https://example.com/video.mp4"}},
		{name: "relative url", part: ContentPart{Type: ContentPartTypeFile, URL: "/tmp/file.txt"}},
		{name: "wrong image mime", part: ContentPart{Type: ContentPartTypeImage, MIMEType: "audio/wav", Data: "aW1hZ2U="}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateContentPart(tt.part); err == nil {
				t.Fatal("ValidateContentPart succeeded, want error")
			}
		})
	}
}
