package anthropicclaude

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// fromContentParts converts OpenAI content formats to Anthropic ContentBlockParamUnion.
func fromContentParts(contentUnion any) ([]anthropic.ContentBlockParamUnion, error) {
	switch v := contentUnion.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(v)}, nil
	case []types.ChatCompletionRequestUserMessageContentPart:
		return fromUserMessageContentParts(v)
	case []types.ChatCompletionRequestAssistantMessageContentPart:
		return fromAssistantMessageContentParts(v)
	case []types.ChatCompletionRequestToolMessageContentPart:
		return fromToolMessageContentParts(v)
	case []types.ChatCompletionRequestSystemMessageContentPart:
		return fromSystemMessageContentParts(v)
	case []types.ChatCompletionRequestDeveloperMessageContentPart:
		return fromDeveloperMessageContentParts(v)
	default:
		return nil, fmt.Errorf("unsupported content format: %T", v)
	}
}

// fromUserMessageContentParts converts user message content parts to Anthropic content blocks.
// User messages support: text, images, audio, files.
func fromUserMessageContentParts(parts []types.ChatCompletionRequestUserMessageContentPart) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for i, partUnion := range parts {
		discriminator, err := partUnion.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of user content part %d: %w", i, err)
		}

		switch discriminator {
		case string(types.ChatCompletionRequestMessageContentPartTextTypeText):
			textPart, err := partUnion.AsChatCompletionRequestMessageContentPartText()
			if err != nil {
				return nil, fmt.Errorf("extract text from user content part %d: %w", i, err)
			}
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartText(textPart))

		case string(types.ImageUrl):
			imagePart, err := partUnion.AsChatCompletionRequestMessageContentPartImage()
			if err != nil {
				return nil, fmt.Errorf("extract image from user content part %d: %w", i, err)
			}
			block, err := fromChatCompletionRequestMessageContentPartImage(imagePart)
			if err != nil {
				return nil, fmt.Errorf("transform image in user content part %d: %w", i, err)
			}
			blocks = append(blocks, block)

		case string(types.InputAudio):
			// Audio transformation: OpenAI's input audio format (base64 data + format) cannot
			// be mapped to Anthropic's audio handling.
			return nil, fmt.Errorf("audio content not supported in user content part %d", i)

		case string(types.File):
			filePart, err := partUnion.AsChatCompletionRequestMessageContentPartFile()
			if err != nil {
				return nil, fmt.Errorf("extract file from user content part %d: %w", i, err)
			}
			block, err := fromChatCompletionRequestMessageContentPartFile(filePart)
			if err != nil {
				return nil, fmt.Errorf("transform file in user content part %d: %w", i, err)
			}
			blocks = append(blocks, block)

		default:
			return nil, fmt.Errorf("content part type %s not supported in user messages", discriminator)
		}
	}
	return blocks, nil
}

// fromAssistantMessageContentParts converts assistant message content parts to Anthropic content blocks.
// Assistant messages support: text, refusals.
func fromAssistantMessageContentParts(parts []types.ChatCompletionRequestAssistantMessageContentPart) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for i, partUnion := range parts {
		discriminator, err := partUnion.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of assistant content part %d: %w", i, err)
		}

		switch discriminator {
		case string(types.ChatCompletionRequestMessageContentPartTextTypeText):
			textPart, err := partUnion.AsChatCompletionRequestMessageContentPartText()
			if err != nil {
				return nil, fmt.Errorf("extract text from assistant content part %d: %w", i, err)
			}
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartText(textPart))

		case string(types.Refusal):
			refusalPart, err := partUnion.AsChatCompletionRequestMessageContentPartRefusal()
			if err != nil {
				return nil, fmt.Errorf("extract refusal from assistant content part %d: %w", i, err)
			}
			// Refusals are model-generated safety responses, treated as text
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartRefusal(refusalPart))

		default:
			return nil, fmt.Errorf("content part type %s not supported in assistant messages", discriminator)
		}
	}
	return blocks, nil
}

// fromToolMessageContentParts converts tool message content parts to Anthropic content blocks.
// Tool messages support: text only.
func fromToolMessageContentParts(parts []types.ChatCompletionRequestToolMessageContentPart) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for i, partUnion := range parts {
		discriminator, err := partUnion.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of tool content part %d: %w", i, err)
		}

		switch discriminator {
		case string(types.ChatCompletionRequestMessageContentPartTextTypeText):
			textPart, err := partUnion.AsChatCompletionRequestMessageContentPartText()
			if err != nil {
				return nil, fmt.Errorf("extract text from tool content part %d: %w", i, err)
			}
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartText(textPart))

		default:
			return nil, fmt.Errorf("content part type %s not supported in tool messages", discriminator)
		}
	}
	return blocks, nil
}

// fromSystemMessageContentParts converts system message content parts to Anthropic content blocks.
// System messages support: text only.
func fromSystemMessageContentParts(parts []types.ChatCompletionRequestSystemMessageContentPart) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for i, partUnion := range parts {
		discriminator, err := partUnion.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of system content part %d: %w", i, err)
		}

		switch discriminator {
		case string(types.ChatCompletionRequestMessageContentPartTextTypeText):
			textPart, err := partUnion.AsChatCompletionRequestMessageContentPartText()
			if err != nil {
				return nil, fmt.Errorf("extract text from system content part %d: %w", i, err)
			}
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartText(textPart))

		default:
			return nil, fmt.Errorf("content part type %s not supported in system messages", discriminator)
		}
	}
	return blocks, nil
}

// fromDeveloperMessageContentParts converts developer message content parts to Anthropic content blocks.
// Developer messages support: text only.
func fromDeveloperMessageContentParts(parts []types.ChatCompletionRequestDeveloperMessageContentPart) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for i, partUnion := range parts {
		discriminator, err := partUnion.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of developer content part %d: %w", i, err)
		}

		switch discriminator {
		case string(types.ChatCompletionRequestMessageContentPartTextTypeText):
			textPart, err := partUnion.AsChatCompletionRequestMessageContentPartText()
			if err != nil {
				return nil, fmt.Errorf("extract text from developer content part %d: %w", i, err)
			}
			blocks = append(blocks, fromChatCompletionRequestMessageContentPartText(textPart))

		default:
			return nil, fmt.Errorf("content part type %s not supported in developer messages", discriminator)
		}
	}
	return blocks, nil
}

// fromChatCompletionRequestMessageContentPartText converts OpenAI text content to Anthropic TextBlock.
func fromChatCompletionRequestMessageContentPartText(textPart types.ChatCompletionRequestMessageContentPartText) anthropic.ContentBlockParamUnion {
	return anthropic.NewTextBlock(textPart.Text)
}

// fromChatCompletionRequestMessageContentPartRefusal converts OpenAI refusal content to Anthropic TextBlock.
// Refusals are preserved as text to maintain conversation continuity in message history.
func fromChatCompletionRequestMessageContentPartRefusal(refusalPart types.ChatCompletionRequestMessageContentPartRefusal) anthropic.ContentBlockParamUnion {
	return anthropic.NewTextBlock(refusalPart.Refusal)
}

// fromChatCompletionRequestMessageContentPartImage converts OpenAI image content to Anthropic format.
func fromChatCompletionRequestMessageContentPartImage(imagePart types.ChatCompletionRequestMessageContentPartImage) (anthropic.ContentBlockParamUnion, error) {
	imageURL := imagePart.ImageUrl.Url

	// Ignore imagePart.ImageUrl.Detail ("low"/"high"/"auto") because Anthropic
	// doesn't have equivalent detail level control in API

	if strings.HasPrefix(imageURL, "data:") {
		// Parse data URL format: data:mime/type;base64,<data>
		parts := strings.Split(imageURL, ",")
		if len(parts) != 2 {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("invalid data URL format, expected data:mime/type;base64,data")
		}

		// Extract media type from data URL prefix
		var mediaType string
		if after, found := strings.CutPrefix(parts[0], "data:"); found {
			if mimeType, _, _ := strings.Cut(after, ";"); mimeType != "" {
				mediaType = mimeType
			}
		}
		if mediaType == "" {
			mediaType = "image/jpeg"
		}

		// Data is already base64 encoded in the data URL
		encodedData := parts[1]

		// Validate it's valid base64
		if _, err := base64.StdEncoding.DecodeString(encodedData); err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("invalid base64 image data: %w", err)
		}

		return anthropic.NewImageBlockBase64(mediaType, encodedData), nil
	} else if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return anthropic.NewImageBlock(anthropic.URLImageSourceParam{
			URL: imageURL,
		}), nil
	} else {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("invalid image URL format: must be http(s):// or data: URI")
	}
}

// fromChatCompletionRequestMessageContentPartFile converts OpenAI file content to Anthropic DocumentBlockParam.
// Supports inline base64 file data (file_data field). File ID references (file_id) are not supported
// as they would require a separate file storage/upload system.
func fromChatCompletionRequestMessageContentPartFile(filePart types.ChatCompletionRequestMessageContentPartFile) (anthropic.ContentBlockParamUnion, error) {
	file := filePart.File

	if file.FileId != nil && *file.FileId != "" {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("file_id references not supported (requires file upload system), only inline file_data is supported")
	}

	if file.FileData == nil || *file.FileData == "" {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("file content requires file_data field (inline base64)")
	}

	decodedFile, err := base64.StdEncoding.DecodeString(*file.FileData)
	if err != nil {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("decode base64 file data: %w", err)
	}

	var filename string
	if file.Filename != nil {
		filename = *file.Filename
	}

	mimeType := detectMIMEType(decodedFile, "", filename, "application/octet-stream")

	if mimeType == "application/pdf" {
		source := anthropic.Base64PDFSourceParam{
			Data: *file.FileData,
		}
		block := anthropic.NewDocumentBlock(source)
		if filename != "" && block.OfDocument != nil {
			block.OfDocument.Title = anthropic.String(filename)
		}
		return block, nil

	} else if strings.HasPrefix(mimeType, "text/") {
		source := anthropic.PlainTextSourceParam{
			Data: string(decodedFile),
		}
		block := anthropic.NewDocumentBlock(source)
		if filename != "" && block.OfDocument != nil {
			block.OfDocument.Title = anthropic.String(filename)
		}
		return block, nil

	} else {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("unsupported file type: %s (only PDF and text files supported by Anthropic)", mimeType)
	}
}

// detectMIMEType determines MIME type using a prioritized fallback chain:
// content sniffing (most reliable) → declared type → filename extension → final fallback.
func detectMIMEType(data []byte, declaredType, filename, fallbackType string) string {
	if detectedMime := http.DetectContentType(data); detectedMime != "application/octet-stream" {
		return detectedMime
	}

	if declaredType != "" && declaredType != "application/octet-stream" {
		return declaredType
	}

	if filename != "" {
		ext := filepath.Ext(strings.ToLower(filename))
		if extMime := mime.TypeByExtension(ext); extMime != "" {
			return extMime
		}
	}

	return fallbackType
}
