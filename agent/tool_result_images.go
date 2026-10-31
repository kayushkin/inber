package agent

import (
	"context"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

// A tool returns text, and some tools also produce images the model should
// see: an MCP server's image content (the bridge's generate_image returns a
// preview of the picture it made). The images travel beside the text rather
// than through Tool.Run's return value, so every wrapper a tool passes
// through (rooting, output truncation, caching) stays as it is: the turn
// loops give each call a holder in its context, a tool attaches its images
// to it, and the loop puts them into the call's tool_result after the text.

// ToolResultImage is one image a tool call returned, base64-encoded.
type ToolResultImage struct {
	MediaType  string
	Base64Data string
}

type toolResultImagesKey struct{}

// toolResultImages holds the images one tool call attached.
type toolResultImages struct {
	mu     sync.Mutex
	images []ToolResultImage
}

// withToolResultImages gives one tool call a holder for its images.
func withToolResultImages(ctx context.Context) (context.Context, *toolResultImages) {
	holder := &toolResultImages{}
	return context.WithValue(ctx, toolResultImagesKey{}, holder), holder
}

// AttachToolResultImage adds an image to the tool result of the call ctx
// belongs to. It reports false when ctx carries no holder (the tool was run
// outside a turn loop), so the caller can say the image went nowhere.
func AttachToolResultImage(ctx context.Context, image ToolResultImage) bool {
	holder, ok := ctx.Value(toolResultImagesKey{}).(*toolResultImages)
	if !ok {
		return false
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.images = append(holder.images, image)
	return true
}

func (h *toolResultImages) attached() []ToolResultImage {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]ToolResultImage(nil), h.images...)
}

// NewToolResultBlockWithImages is anthropic.NewToolResultBlock with the
// call's images after its text.
func NewToolResultBlockWithImages(toolUseID, text string, isError bool, images []ToolResultImage) anthropic.ContentBlockParamUnion {
	block := anthropic.NewToolResultBlock(toolUseID, text, isError)
	for _, image := range images {
		block.OfToolResult.Content = append(block.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
			OfImage: &anthropic.ImageBlockParam{Source: anthropic.ImageBlockParamSourceUnion{
				OfBase64: &anthropic.Base64ImageSourceParam{
					Data:      image.Base64Data,
					MediaType: anthropic.Base64ImageSourceMediaType(image.MediaType),
				},
			}},
		})
	}
	return block
}

// ToolResultImagesFor gives a tool call outside this package (the OpenAI turn
// loop in engine) a holder, and returns a function that reads what the call
// attached.
func ToolResultImagesFor(ctx context.Context) (context.Context, func() []ToolResultImage) {
	callCtx, holder := withToolResultImages(ctx)
	return callCtx, holder.attached
}
