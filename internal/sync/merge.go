package sync

import (
	"strings"

	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/contentful"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/linkedin"
)

// Merge combines existing Contentful testimonials with newly scraped
// LinkedIn recommendations. Deduplication uses a composite key of
// normalized (lowercased, trimmed) name + company.
func Merge(existing []contentful.Testimonial, scraped []linkedin.Recommendation) ([]contentful.Testimonial, int) {
	seen := make(map[string]bool, len(existing))
	for _, t := range existing {
		seen[dedupeKey(t.Name, t.Company)] = true
	}

	result := make([]contentful.Testimonial, len(existing))
	copy(result, existing)

	newCount := 0
	for _, rec := range scraped {
		key := dedupeKey(rec.Name, rec.Company)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, contentful.Testimonial{
			Name:      rec.Name,
			Role:      rec.Role,
			Company:   rec.Company,
			Quote:     rec.Quote,
			AvatarURL: rec.AvatarURL,
		})
		newCount++
	}

	return result, newCount
}

func dedupeKey(name, company string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	c := strings.ToLower(strings.TrimSpace(company))
	return n + "|" + c
}
