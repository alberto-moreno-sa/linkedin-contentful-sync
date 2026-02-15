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

	servicekit "github.com/alberto-moreno-sa/go-service-kit/contentful"
)

// Client embeds the SDK client and adds testimonial-specific methods.
type Client struct {
	*servicekit.Client
}

// NewClient creates a new Contentful client with SDK and testimonial support.
func NewClient(spaceID, token string) *Client {
	return &Client{
		Client: servicekit.NewClient(spaceID, token),
	}
}

// GetTestimonials fetches the testimonials siteSection entry.
func (c *Client) GetTestimonials(ctx context.Context) (*TestimonialsResult, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries", servicekit.CMABaseURL, c.SpaceID)

	params := url.Values{}
	params.Set("content_type", "siteSection")
	params.Set("fields.sectionId", "testimonials")
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CMA query failed (%d): %s", resp.StatusCode, string(body))
	}

	var result servicekit.EntriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Items) == 0 {
		return &TestimonialsResult{}, nil
	}

	entry := result.Items[0]

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
func (c *Client) UpdateTestimonials(ctx context.Context, result *TestimonialsResult, testimonials []Testimonial) (int, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries/%s",
		servicekit.CMABaseURL, c.SpaceID, result.EntryID)

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
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")
	req.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", result.Version))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("CMA update failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var updated servicekit.EntryItem
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return 0, fmt.Errorf("decode update response: %w", err)
	}

	return updated.Sys.Version, nil
}

// CreateTestimonials creates a new siteSection entry for testimonials.
func (c *Client) CreateTestimonials(ctx context.Context, testimonials []Testimonial) (string, int, error) {
	endpoint := fmt.Sprintf("%s/spaces/%s/environments/master/entries", servicekit.CMABaseURL, c.SpaceID)

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
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")
	req.Header.Set("X-Contentful-Content-Type", "siteSection")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("CMA create failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var created servicekit.EntryItem
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", 0, fmt.Errorf("decode create response: %w", err)
	}

	return created.Sys.ID, created.Sys.Version, nil
}

// UploadAvatar downloads an image from imageURL, uploads it to Contentful as an asset,
// processes and publishes it, and returns the CDN URL.
func (c *Client) UploadAvatar(ctx context.Context, imageURL, name string) (string, error) {
	imgReq, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create image request: %w", err)
	}
	imgResp, err := c.HTTPClient.Do(imgReq)
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

	uploadEndpoint := fmt.Sprintf("https://upload.contentful.com/spaces/%s/uploads", c.SpaceID)
	uploadReq, err := http.NewRequestWithContext(ctx, "POST", uploadEndpoint, bytes.NewReader(imgData))
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("Authorization", "Bearer "+c.Token)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")

	uploadResp, err := c.HTTPClient.Do(uploadReq)
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

	assetEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets", servicekit.CMABaseURL, c.SpaceID)
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
	assetReq.Header.Set("Authorization", "Bearer "+c.Token)
	assetReq.Header.Set("Content-Type", "application/vnd.contentful.management.v1+json")

	assetResp, err := c.HTTPClient.Do(assetReq)
	if err != nil {
		return "", fmt.Errorf("create asset: %w", err)
	}
	defer assetResp.Body.Close()

	if assetResp.StatusCode != 201 {
		body, _ := io.ReadAll(assetResp.Body)
		return "", fmt.Errorf("create asset failed (%d): %s", assetResp.StatusCode, string(body))
	}

	var assetResult servicekit.EntryItem
	if err := json.NewDecoder(assetResp.Body).Decode(&assetResult); err != nil {
		return "", fmt.Errorf("decode asset: %w", err)
	}

	processEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets/%s/files/en-US/process",
		servicekit.CMABaseURL, c.SpaceID, assetResult.Sys.ID)
	processReq, err := http.NewRequestWithContext(ctx, "PUT", processEndpoint, nil)
	if err != nil {
		return "", err
	}
	processReq.Header.Set("Authorization", "Bearer "+c.Token)
	processReq.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", assetResult.Sys.Version))

	processResp, err := c.HTTPClient.Do(processReq)
	if err != nil {
		return "", fmt.Errorf("process asset: %w", err)
	}
	processResp.Body.Close()

	if processResp.StatusCode != 204 {
		return "", fmt.Errorf("process asset returned %d", processResp.StatusCode)
	}

	assetGetEndpoint := fmt.Sprintf("%s/spaces/%s/environments/master/assets/%s",
		servicekit.CMABaseURL, c.SpaceID, assetResult.Sys.ID)

	var cdnURL string
	var assetVersion int
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)

		getReq, err := http.NewRequestWithContext(ctx, "GET", assetGetEndpoint, nil)
		if err != nil {
			return "", err
		}
		getReq.Header.Set("Authorization", "Bearer "+c.Token)

		getResp, err := c.HTTPClient.Do(getReq)
		if err != nil {
			continue
		}

		var polled struct {
			Sys    servicekit.EntrySys    `json:"sys"`
			Fields map[string]interface{} `json:"fields"`
		}
		json.NewDecoder(getResp.Body).Decode(&polled)
		getResp.Body.Close()

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

	if err := c.publishAsset(ctx, assetResult.Sys.ID, assetVersion); err != nil {
		return "", fmt.Errorf("publish asset: %w", err)
	}

	return cdnURL, nil
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
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
		servicekit.CMABaseURL, c.SpaceID, assetID)

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("X-Contentful-Version", fmt.Sprintf("%d", version))

	resp, err := c.HTTPClient.Do(req)
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
