package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/png" // Register PNG decoder

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/ai8future/airborne/internal/provider"
)

const (
	defaultOpenAIModel = "dall-e-3"
)

func (c *Client) generateOpenAI(ctx context.Context, req *ImageRequest) (provider.GeneratedImage, error) {
	if req.OpenAIAPIKey == "" {
		return provider.GeneratedImage{}, fmt.Errorf("openai API key not configured for image generation")
	}

	client := openai.NewClient(option.WithAPIKey(req.OpenAIAPIKey))

	model := req.Config.GetModel()
	if model == "" {
		model = defaultOpenAIModel
	}

	resp, err := client.Images.Generate(ctx, openai.ImageGenerateParams{
		Prompt:         req.Prompt,
		Model:          openai.ImageModel(model),
		N:              openai.Int(1),
		Size:           openai.ImageGenerateParamsSize1024x1024,
		ResponseFormat: openai.ImageGenerateParamsResponseFormatB64JSON,
	})
	if err != nil {
		return provider.GeneratedImage{}, fmt.Errorf("openai image generation: %w", err)
	}

	if len(resp.Data) == 0 {
		return provider.GeneratedImage{}, fmt.Errorf("no image returned from openai")
	}

	imgData, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return provider.GeneratedImage{}, fmt.Errorf("decode image: %w", err)
	}

	width, height := getImageDimensions(imgData)

	return provider.GeneratedImage{
		Data:     imgData,
		MIMEType: "image/png",
		Prompt:   req.Prompt,
		AltText:  truncateForAlt(req.Prompt, 125),
		Width:    width,
		Height:   height,
	}, nil
}

// getImageDimensions decodes image metadata to get dimensions.
func getImageDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
