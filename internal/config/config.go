package config

import (
	"fmt"
	"os"
)

type Config struct {
	SpaceID        string
	CMAToken       string
	LinkedInCookie string
}

// Load loads all config including LinkedIn cookie (for scrape command).
func Load() (*Config, error) {
	cfg, err := LoadContentful()
	if err != nil {
		return nil, err
	}

	cfg.LinkedInCookie = os.Getenv("LINKEDIN_COOKIE")
	if cfg.LinkedInCookie == "" {
		return nil, fmt.Errorf("LINKEDIN_COOKIE (li_at value) is required")
	}

	return cfg, nil
}

// LoadContentful loads only Contentful config (for list command).
func LoadContentful() (*Config, error) {
	cfg := &Config{
		SpaceID:  os.Getenv("CONTENTFUL_SPACE_ID"),
		CMAToken: os.Getenv("CONTENTFUL_CMA_TOKEN"),
	}

	if cfg.SpaceID == "" {
		return nil, fmt.Errorf("CONTENTFUL_SPACE_ID is required")
	}
	if cfg.CMAToken == "" {
		return nil, fmt.Errorf("CONTENTFUL_CMA_TOKEN is required")
	}

	return cfg, nil
}
