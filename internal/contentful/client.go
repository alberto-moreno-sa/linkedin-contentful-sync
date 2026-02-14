package contentful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const cmaBaseURL = "https://api.contentful.com"

type Client struct {
	spaceID    string
	token      string
	httpClient *http.Client
}

func NewClient(spaceID, token string) *Client {
	return &Client{
		spaceID:    spaceID,
		token:      token,
		httpClient: &http.Client{},
	}
}

// GetTestimonials fetches the testimonials siteSection entry.
func (c *Client) GetTestimonials(ctx context.Context) (*TestimonialsResult, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries", cmaBaseURL, c.spaceID)

	params := url.Values{}
	params.Set("content_type", "siteSection")
	params.Set("fields.sectionId", "testimonials")
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CMA query failed (%d): %s", resp.StatusCode, string(body))
	}

	var result entriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no entry found with sectionId=testimonials")
	}

	entry := result.Items[0]

	// Extract testimonials from the locale-wrapped content field
	contentField, ok := entry.Fields["content"]
	if !ok {
		return &TestimonialsResult{
			EntryID:   entry.Sys.ID,
			Version:   entry.Sys.Version,
			RawFields: entry.Fields,
		}, fmt.Errorf("entry has no 'content' field")
	}

	localeMap, ok := contentField.(map[string]interface{})
	if !ok {
		return &TestimonialsResult{
			EntryID:   entry.Sys.ID,
			Version:   entry.Sys.Version,
			RawFields: entry.Fields,
		}, fmt.Errorf("content field is not locale-wrapped")
	}

	// Try en-US locale (Contentful default)
	rawContent, ok := localeMap["en-US"]
	if !ok {
		for _, v := range localeMap {
			rawContent = v
			break
		}
	}

	contentBytes, err := json.Marshal(rawContent)
	if err != nil {
		return nil, fmt.Errorf("marshal content: %w", err)
	}

	var testimonials []Testimonial
	if err := json.Unmarshal(contentBytes, &testimonials); err != nil {
		return nil, fmt.Errorf("unmarshal testimonials: %w", err)
	}

	return &TestimonialsResult{
		Testimonials: testimonials,
		EntryID:      entry.Sys.ID,
		Version:      entry.Sys.Version,
		RawFields:    entry.Fields,
	}, nil
}

// UpdateTestimonials updates the testimonials entry using the fetch-mutate-put pattern.
// It preserves all existing fields and only replaces the content field.
func (c *Client) UpdateTestimonials(ctx context.Context, result *TestimonialsResult, testimonials []Testimonial) (int, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries/%s",
		cmaBaseURL, c.spaceID, result.EntryID)

	// Clone raw fields and replace only the content
	fields := make(map[string]interface{})
	for k, v := range result.RawFields {
		fields[k] = v
	}
	fields["content"] = map[string]interface{}{
		"en-US": testimonials,
	}

	body := map[string]interface{}{
		"fields": fields,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")
	req.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", result.Version))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("CMA update failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var updated entryItem
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return 0, fmt.Errorf("decode update response: %w", err)
	}

	return updated.Sys.Version, nil
}

// PublishEntry publishes a Contentful entry.
func (c *Client) PublishEntry(ctx context.Context, entryID string, version int) error {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries/%s/published",
		cmaBaseURL, c.spaceID, entryID)

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", version))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CMA publish failed (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}
