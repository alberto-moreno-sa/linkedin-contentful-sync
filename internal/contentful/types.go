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
