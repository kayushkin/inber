package conversation

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// noisyPNGBase64 makes a PNG of the given size whose pixels are seeded noise,
// so it does not compress and its base64 text is large, as a real picture's is.
func noisyPNGBase64(t *testing.T, width, height int) string {
	t.Helper()
	random := rand.New(rand.NewSource(1))
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, color.RGBA{uint8(random.Intn(256)), uint8(random.Intn(256)), uint8(random.Intn(256)), 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func toolResultWithImage(text, base64Data string) anthropic.ContentBlockParamUnion {
	block := anthropic.NewToolResultBlock("toolu_image", text, false)
	block.OfToolResult.Content = append(block.OfToolResult.Content, anthropic.ToolResultBlockParamContentUnion{
		OfImage: &anthropic.ImageBlockParam{Source: anthropic.ImageBlockParamSourceUnion{
			OfBase64: &anthropic.Base64ImageSourceParam{Data: base64Data, MediaType: "image/png"},
		}},
	})
	return block
}

// A tool result holding an image is priced by the image's pixels, not its
// base64 text. Before, a 512x512 noisy PNG (about 1 MB of base64) priced at
// hundreds of thousands of tokens, past the emergency flush on its own.
func TestToolResultImageIsPricedByPixelsNotBase64(t *testing.T) {
	data := noisyPNGBase64(t, 512, 512)
	message := anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{toolResultWithImage("saved image", data)},
	}

	estimate := EstimateMessageTokens(message)
	pixelTokens := imageTokensForSize(512, 512) // 350
	if estimate < pixelTokens {
		t.Fatalf("estimate %d is below the image's own pixel price %d", estimate, pixelTokens)
	}
	if estimate > pixelTokens+100 {
		t.Fatalf("estimate %d still counts the base64 text (%d bytes); want about %d", estimate, len(data), pixelTokens)
	}
}

// The text beside an image is still counted.
func TestToolResultTextBesideAnImageIsStillCounted(t *testing.T) {
	data := noisyPNGBase64(t, 16, 16)
	short := EstimateMessageTokens(anthropic.MessageParam{Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{toolResultWithImage("x", data)}})
	long := EstimateMessageTokens(anthropic.MessageParam{Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{toolResultWithImage(strings.Repeat("word ", 4000), data)}})
	if long-short < 3000 {
		t.Fatalf("20 KB of text beside the image added only %d tokens", long-short)
	}
}

// Pricing must not change the block it prices.
func TestPricingAnImageLeavesTheBlockWhole(t *testing.T) {
	data := noisyPNGBase64(t, 8, 8)
	block := toolResultWithImage("saved image", data)
	_ = estimateContentBlockTokens(block)
	if countToolResultImages(block.OfToolResult.Content) != 1 || block.OfToolResult.Content[1].OfImage.Source.OfBase64.Data != data {
		t.Fatal("pricing removed or changed the image in the original block")
	}
}

func TestImageTokensForSize(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		want          int
	}{
		{"small image at its own size", 750, 1, 1},
		{"1024 square", 1024, 1024, 1399},
		{"long edge shrunk to the limit", imageLongEdgeLimitPixels * 2, imageLongEdgeLimitPixels, 4424},
		{"unknown size charged the ceiling", 0, 0, imageTokenCeiling},
	}
	for _, c := range cases {
		if got := imageTokensForSize(c.width, c.height); got != c.want {
			t.Errorf("%s: imageTokensForSize(%d, %d) = %d, want %d", c.name, c.width, c.height, got, c.want)
		}
	}
}

// An image whose header cannot be read is charged the ceiling, never its text.
func TestUnreadableImageIsChargedTheCeiling(t *testing.T) {
	block := toolResultWithImage("", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("not an image", 50000))))
	if got := estimateContentBlockTokens(block); got < imageTokenCeiling || got > imageTokenCeiling+100 {
		t.Fatalf("unreadable image priced at %d, want about the ceiling %d", got, imageTokenCeiling)
	}
}

// A result whose text is a short caption used to be kept, image and all, until
// the drop age. It is now summarized at the summary age like any large result,
// and the marker says an image went.
func TestShortCaptionDoesNotKeepAnImagePastKeepFull(t *testing.T) {
	cfg := ManagementConfigForRole("default")
	block := toolResultWithImage("saved image", noisyPNGBase64(t, 8, 8))

	kept, changed := pruneToolResult(block, cfg.ToolResultKeepFull-1, cfg, "generate_image")
	if changed || countToolResultImages(kept.OfToolResult.Content) != 1 {
		t.Fatal("a recent result with an image lost its image")
	}

	pruned, changed := pruneToolResult(block, cfg.ToolResultKeepFull, cfg, "generate_image")
	if !changed || countToolResultImages(pruned.OfToolResult.Content) != 0 {
		t.Fatalf("at the summary age the image was kept")
	}
	summary := pruned.OfToolResult.Content[0].OfText.Text
	if !strings.Contains(summary, "1 image dropped") || !strings.Contains(summary, "saved image") || !strings.Contains(summary, "generate_image") {
		t.Fatalf("summary does not say the image went, or lost the caption: %q", summary)
	}
}

func TestLongResultSummaryAndDropMarkerSayAnImageWent(t *testing.T) {
	cfg := ManagementConfigForRole("default")
	data := noisyPNGBase64(t, 8, 8)
	long := toolResultWithImage(strings.Repeat("line of output\n", 20), data)
	long.OfToolResult.Content = append(long.OfToolResult.Content, long.OfToolResult.Content[1])

	summarized, _ := pruneToolResult(long, cfg.ToolResultKeepFull, cfg, "browser")
	if text := summarized.OfToolResult.Content[0].OfText.Text; !strings.HasSuffix(text, "[2 images dropped]") {
		t.Fatalf("summary does not say two images went: %q", text)
	}

	dropped, _ := pruneToolResult(toolResultWithImage("saved image", data), cfg.ToolResultDrop, cfg, "generate_image")
	if text := dropped.OfToolResult.Content[0].OfText.Text; text != "[generate_image: dropped - too old, 11 bytes, 1 image dropped]" {
		t.Fatalf("drop marker = %q", text)
	}
}

// A result with no image is pruned exactly as before.
func TestShortTextOnlyResultIsStillKept(t *testing.T) {
	cfg := ManagementConfigForRole("default")
	block := anthropic.NewToolResultBlock("toolu_text", "ok", false)
	if _, changed := pruneToolResult(block, cfg.ToolResultKeepFull, cfg, "shell"); changed {
		t.Fatal("a short text-only result was pruned")
	}
	dropped, _ := pruneToolResult(block, cfg.ToolResultDrop, cfg, "shell")
	if text := dropped.OfToolResult.Content[0].OfText.Text; text != "[shell: dropped - too old, 2 bytes]" {
		t.Fatalf("drop marker = %q", text)
	}
}
