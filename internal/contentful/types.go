package contentful

// Testimonial matches the JSON structure in the Contentful siteSection content field.
type Testimonial struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Company     string `json:"company"`
	Quote       string `json:"quote"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	LinkedInURL string `json:"linkedInUrl,omitempty"`
}

// TestimonialsResult holds the fetched testimonials along with entry metadata
// needed for the fetch-mutate-put update pattern.
type TestimonialsResult struct {
	Testimonials []Testimonial
	EntryID      string
	Version      int
	RawFields    map[string]interface{}
}

type entriesResponse struct {
	Items []entryItem `json:"items"`
	Total int         `json:"total"`
}

type entryItem struct {
	Sys    entrySys               `json:"sys"`
	Fields map[string]interface{} `json:"fields"`
}

type entrySys struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}
