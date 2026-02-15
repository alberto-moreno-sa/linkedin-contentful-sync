package translate

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// ToEnglish translates the given text to English using Google Gemini.
func ToEnglish(ctx context.Context, apiKey, text string) (string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("gemini client: %w", err)
	}

	result, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(text), &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: "Translate the following text to English. Return only the translated text, nothing else."},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("gemini generate: %w", err)
	}

	return strings.TrimSpace(result.Text()), nil
}
