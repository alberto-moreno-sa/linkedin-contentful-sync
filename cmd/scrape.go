package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/config"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/contentful"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/linkedin"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/sync"
	"github.com/spf13/cobra"
)

var profileFlag string

var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape LinkedIn recommendations and sync to Contentful",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}

		if profileFlag == "" {
			return fmt.Errorf("--profile flag is required")
		}

		// Step 1: Scrape LinkedIn
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		log.Println("Scraping LinkedIn recommendations...")
		scraped, err := linkedin.Scrape(ctx, profileFlag, cfg.LinkedInCookie, verbose)
		if err != nil {
			return fmt.Errorf("scrape: %w", err)
		}
		log.Printf("Found %d recommendations\n", len(scraped))

		if len(scraped) == 0 {
			log.Println("No recommendations found. Selectors may need updating.")
			return nil
		}

		// Step 2: Fetch existing testimonials from Contentful
		cmaClient := contentful.NewClient(cfg.SpaceID, cfg.CMAToken)
		result, err := cmaClient.GetTestimonials(ctx)
		if err != nil {
			return fmt.Errorf("contentful fetch: %w", err)
		}
		log.Printf("Existing testimonials: %d\n", len(result.Testimonials))

		// Step 3: Merge
		merged, newCount := sync.Merge(result.Testimonials, scraped)
		if newCount == 0 {
			log.Println("No new recommendations to add. Everything is up to date.")
			return nil
		}
		log.Printf("Adding %d new recommendations (total: %d)\n", newCount, len(merged))

		// Step 4: Update + Publish
		newVersion, err := cmaClient.UpdateTestimonials(ctx, result, merged)
		if err != nil {
			return fmt.Errorf("contentful update: %w", err)
		}

		err = cmaClient.PublishEntry(ctx, result.EntryID, newVersion)
		if err != nil {
			return fmt.Errorf("contentful publish: %w", err)
		}

		log.Println("Successfully synced and published.")
		return nil
	},
}

func init() {
	scrapeCmd.Flags().StringVar(&profileFlag, "profile", "", "LinkedIn username (e.g. alberthiggs)")
	rootCmd.AddCommand(scrapeCmd)
}
