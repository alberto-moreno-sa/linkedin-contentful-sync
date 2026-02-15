package contentful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
		return &TestimonialsResult{}, nil
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

// CreateTestimonials creates a new siteSection entry for testimonials.
func (c *Client) CreateTestimonials(ctx context.Context, testimonials []Testimonial) (string, int, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries", cmaBaseURL, c.spaceID)

	body := map[string]interface{}{
		"fields": map[string]interface{}{
			"sectionId": map[string]interface{}{"en-US": "testimonials"},
			"title":     map[string]interface{}{"en-US": "Testimonials"},
			"content":   map[string]interface{}{"en-US": testimonials},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", 0, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")
	req.Header.Set("X-Contentful-Content-Type", "siteSection")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("CMA create failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var created entryItem
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", 0, fmt.Errorf("decode create response: %w", err)
	}

	return created.Sys.ID, created.Sys.Version, nil
}

// UploadAvatar downloads an image from imageURL, uploads it to Contentful as an asset,
// processes and publishes it, and returns the CDN URL.
func (c *Client) UploadAvatar(ctx context.Context, imageURL, name string) (string, error) {
	// Step 1: Download the image
	imgReq, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create image request: %w", err)
	}
	imgResp, err := c.httpClient.Do(imgReq)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != 200 {
		return "", fmt.Errorf("download image returned %d", imgResp.StatusCode)
	}

	imgData, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	contentType := imgResp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	fileName := slugify(name) + extForContentType(contentType)

	// Step 2: Upload binary to Contentful (upload API uses a different host)
	uploadEndpoint := fmt.Sprintf("https://upload.contentful.com/spaces/%s/uploads", c.spaceID)
	uploadReq, err := http.NewRequestWithContext(ctx, "POST", uploadEndpoint, bytes.NewReader(imgData))
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("Authorization", "Bearer "+c.token)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")

	uploadResp, err := c.httpClient.Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("upload binary: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != 201 {
		body, _ := io.ReadAll(uploadResp.Body)
		return "", fmt.Errorf("upload failed (%d): %s", uploadResp.StatusCode, string(body))
	}

	var uploadResult struct {
		Sys struct {
			ID string `json:"id"`
		} `json:"sys"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadResult); err != nil {
		return "", fmt.Errorf("decode upload: %w", err)
	}

	// Step 3: Create asset referencing the upload
	assetEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets", cmaBaseURL, c.spaceID)
	assetBody := map[string]interface{}{
		"fields": map[string]interface{}{
			"title": map[string]interface{}{"en-US": name + " avatar"},
			"file": map[string]interface{}{
				"en-US": map[string]interface{}{
					"contentType": contentType,
					"fileName":    fileName,
					"uploadFrom": map[string]interface{}{
						"sys": map[string]interface{}{
							"type":     "Link",
							"linkType": "Upload",
							"id":       uploadResult.Sys.ID,
						},
					},
				},
			},
		},
	}

	assetBytes, err := json.Marshal(assetBody)
	if err != nil {
		return "", fmt.Errorf("marshal asset: %w", err)
	}

	assetReq, err := http.NewRequestWithContext(ctx, "POST", assetEndpoint, bytes.NewReader(assetBytes))
	if err != nil {
		return "", err
	}
	assetReq.Header.Set("Authorization", "Bearer "+c.token)
	assetReq.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")

	assetResp, err := c.httpClient.Do(assetReq)
	if err != nil {
		return "", fmt.Errorf("create asset: %w", err)
	}
	defer assetResp.Body.Close()

	if assetResp.StatusCode != 201 {
		body, _ := io.ReadAll(assetResp.Body)
		return "", fmt.Errorf("create asset failed (%d): %s", assetResp.StatusCode, string(body))
	}

	var assetResult entryItem
	if err := json.NewDecoder(assetResp.Body).Decode(&assetResult); err != nil {
		return "", fmt.Errorf("decode asset: %w", err)
	}

	// Step 4: Process the asset
	processEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets/%s/files/en-US/process",
		cmaBaseURL, c.spaceID, assetResult.Sys.ID)
	processReq, err := http.NewRequestWithContext(ctx, "PUT", processEndpoint, nil)
	if err != nil {
		return "", err
	}
	processReq.Header.Set("Authorization", "Bearer "+c.token)
	processReq.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", assetResult.Sys.Version))

	processResp, err := c.httpClient.Do(processReq)
	if err != nil {
		return "", fmt.Errorf("process asset: %w", err)
	}
	processResp.Body.Close()

	if processResp.StatusCode != 204 {
		return "", fmt.Errorf("process asset returned %d", processResp.StatusCode)
	}

	// Step 5: Poll until processed (file.url appears)
	assetGetEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets/%s",
		cmaBaseURL, c.spaceID, assetResult.Sys.ID)

	var cdnURL string
	var assetVersion int
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)

		getReq, err := http.NewRequestWithContext(ctx, "GET", assetGetEndpoint, nil)
		if err != nil {
			return "", err
		}
		getReq.Header.Set("Authorization", "Bearer "+c.token)

		getResp, err := c.httpClient.Do(getReq)
		if err != nil {
			continue
		}

		var polled struct {
			Sys    entrySys               `json:"sys"`
			Fields map[string]interface{} `json:"fields"`
		}
		json.NewDecoder(getResp.Body).Decode(&polled)
		getResp.Body.Close()

		// Check if file has been processed (url field appears)
		if fileField, ok := polled.Fields["file"]; ok {
			if localeMap, ok := fileField.(map[string]interface{}); ok {
				if enUS, ok := localeMap["en-US"].(map[string]interface{}); ok {
					if u, ok := enUS["url"].(string); ok && u != "" {
						cdnURL = "https:" + u
						assetVersion = polled.Sys.Version
						break
					}
				}
			}
		}
	}

	if cdnURL == "" {
		return "", fmt.Errorf("asset processing timed out for %s", name)
	}

	// Step 6: Publish the asset
	if err := c.publishAsset(ctx, assetResult.Sys.ID, assetVersion); err != nil {
		return "", fmt.Errorf("publish asset: %w", err)
	}

	return cdnURL, nil
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non-alphanumeric chars except hyphens
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func extForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}

func (c *Client) publishAsset(ctx context.Context, assetID string, version int) error {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets/%s/published",
		cmaBaseURL, c.spaceID, assetID)

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
		return fmt.Errorf("CMA asset publish failed (%d): %s", resp.StatusCode, string(body))
	}

	return nil
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
