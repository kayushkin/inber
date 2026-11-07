package conversation

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"  // registers GIF for image.DecodeConfig
	_ "image/jpeg" // registers JPEG for image.DecodeConfig
	_ "image/png"  // registers PNG for image.DecodeConfig

	"github.com/anthropics/anthropic-sdk-go"
)

// An image costs the model its pixels, not its bytes. Anthropic's vision docs
// give the cost as about width*height/750 tokens, after the API shrinks the
// image so its long edge fits its limit; a base64 image priced as text runs
// about 40 times higher (a 200 KB base64 PNG estimated at 68,329 tokens when
// the API charges it about 1,600). Priced that way, two generated-image
// previews were enough to pass the emergency flush's TokenBudget*2.
//
// These numbers come from the docs, not from a measurement on this host.
// imageLongEdgeLimitPixels is the largest long edge any current model accepts,
// and the estimate charges an image as if it were kept at that size. For a
// model that shrinks further the estimate runs high, the safe direction for an
// overflow guard.
const (
	imagePixelsPerToken      = 750
	imageLongEdgeLimitPixels = 2576
)

// imageTokenCeiling is what the largest image the API keeps can cost. An
// image whose size cannot be read from its header (a URL source, WebP, or
// data that is not an image) is charged this much: the guard cannot know its
// size, and charging the most it could cost keeps the guard on the safe side
// without pricing it as text.
const imageTokenCeiling = imageLongEdgeLimitPixels * imageLongEdgeLimitPixels / imagePixelsPerToken

// imageTokensForSize prices an image of width by height pixels.
func imageTokensForSize(width, height int) int {
	if width <= 0 || height <= 0 {
		return imageTokenCeiling
	}
	longEdge := max(width, height)
	if longEdge > imageLongEdgeLimitPixels {
		scale := float64(imageLongEdgeLimitPixels) / float64(longEdge)
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))
	}
	tokens := (width*height + imagePixelsPerToken - 1) / imagePixelsPerToken
	return min(tokens, imageTokenCeiling)
}

// estimateImageTokens prices one image block by its pixels, reading only the
// header for its size.
func estimateImageTokens(img *anthropic.ImageBlockParam) int {
	if img == nil || img.Source.OfBase64 == nil {
		return imageTokenCeiling
	}
	decoded, err := base64.StdEncoding.DecodeString(img.Source.OfBase64.Data)
	if err != nil {
		return imageTokenCeiling
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return imageTokenCeiling
	}
	return imageTokensForSize(config.Width, config.Height)
}

// estimateContentBlockTokens prices one content block. An image, on its own or
// inside a tool result, is priced by its pixels, and everything else in the
// block is priced as the JSON the SDK sends, as estimateMarshalled does.
//
// The block is copied, never changed: the copy has its images taken out
// before it is marshalled, so the image data is not counted twice.
//
// This is a token count. A byte-size limit (a 413) is a different measure, and
// the base64 bytes are the right unit there; nothing here should be used for
// that.
func estimateContentBlockTokens(block anthropic.ContentBlockParamUnion) int {
	if block.OfImage != nil {
		return estimateImageTokens(block.OfImage)
	}
	if block.OfToolResult == nil {
		return estimateMarshalled(block)
	}

	imageTokens := 0
	withoutImages := *block.OfToolResult
	withoutImages.Content = nil
	for _, part := range block.OfToolResult.Content {
		if part.OfImage != nil {
			imageTokens += estimateImageTokens(part.OfImage)
			continue
		}
		withoutImages.Content = append(withoutImages.Content, part)
	}
	return imageTokens + estimateMarshalled(anthropic.ContentBlockParamUnion{OfToolResult: &withoutImages})
}

// countToolResultImages counts the images in a tool result's content.
func countToolResultImages(content []anthropic.ToolResultBlockParamContentUnion) int {
	count := 0
	for _, part := range content {
		if part.OfImage != nil {
			count++
		}
	}
	return count
}
