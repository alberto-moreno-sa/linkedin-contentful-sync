package linkedin

// Recommendation represents a single LinkedIn recommendation as scraped.
type Recommendation struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Company   string `json:"company"`
	Quote     string `json:"quote"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}
