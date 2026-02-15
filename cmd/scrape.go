package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	servicekit "github.com/alberto-moreno-sa/go-service-kit/contentful"
	"github.com/alberto-moreno-sa/go-service-kit/gemini"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/config"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/contentful"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/linkedin"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/sync"
	"github.com/spf13/cobra"
)

var profileFlag string
var translateFlag bool
var forceFlag bool

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

		// Step 1.5: Translate quotes to English if requested
		if translateFlag {
			if cfg.GeminiAPIKey == "" {
				return fmt.Errorf("GEMINI_API_KEY is required when using --translate")
			}
			log.Println("Translating quotes to English...")
			for i := range scraped {
				translated, err := gemini.Translate(ctx, cfg.GeminiAPIKey, scraped[i].Quote, "English")
				if err != nil {
					log.Printf("WARNING: translation failed for %s: %v", scraped[i].Name, err)
					continue
				}
				log.Printf("Translated quote for %s", scraped[i].Name)
				scraped[i].Quote = translated
			}
		}

		// Step 2: Fetch existing testimonials from Contentful
		cmaClient := contentful.NewClient(cfg.SpaceID, cfg.CMAToken)
		result, err := cmaClient.GetTestimonials(ctx)
		if err != nil {
			return fmt.Errorf("contentful fetch: %w", err)
		}
		log.Printf("Existing testimonials: %d\n", len(result.Testimonials))

		// Step 3: Merge (or replace if --force)
		var merged []contentful.Testimonial
		var newIndices []int

		if forceFlag {
			log.Println("Force mode: replacing all testimonials")
			for i, rec := range scraped {
				newIndices = append(newIndices, i)
				merged = append(merged, contentful.Testimonial{
					Name:        rec.Name,
					Role:        rec.Role,
					Company:     rec.Company,
					Quote:       rec.Quote,
					AvatarURL:   rec.AvatarURL,
					LinkedInURL: rec.LinkedInURL,
				})
			}
		} else {
			merged, newIndices = sync.Merge(result.Testimonials, scraped)
			if len(newIndices) == 0 {
				log.Println("No new recommendations to add. Everything is up to date.")
				return nil
			}
		}
		log.Printf("Syncing %d recommendations (new: %d)\n", len(merged), len(newIndices))

		// Step 3.5: Upload avatars for new recommendations
		for _, idx := range newIndices {
			t := &merged[idx]
			if t.AvatarURL == "" {
				continue
			}
			log.Printf("Uploading avatar for %s...", t.Name)
			cdnURL, err := cmaClient.UploadAvatar(ctx, t.AvatarURL, t.Name)
			if err != nil {
				log.Printf("WARNING: avatar upload failed for %s: %v", t.Name, err)
				t.AvatarURL = ""
				continue
			}
			t.AvatarURL = cdnURL
			log.Printf("Avatar uploaded for %s: ok", t.Name)
		}

		// Step 4: Create or Update + Publish
		var entryID string
		var newVersion int

		if result.EntryID == "" {
			// Entry doesn't exist yet — create it
			log.Println("Creating new testimonials entry in Contentful...")
			entryID, newVersion, err = cmaClient.CreateTestimonials(ctx, merged)
			if err != nil {
				return fmt.Errorf("contentful create: %w", err)
			}
		} else {
			// Entry exists — update it
			entryID = result.EntryID
			newVersion, err = cmaClient.UpdateTestimonials(ctx, result, merged)
			if err != nil {
				return fmt.Errorf("contentful update: %w", err)
			}
		}

		err = cmaClient.PublishEntry(ctx, entryID, newVersion)
		if err != nil {
			return fmt.Errorf("contentful publish: %w", err)
		}

		log.Println("Successfully synced and published.")

		// Step 5: Record build log
		log.Println("Recording build log...")
		const serviceName = "linkedin-contentful-sync"
		triggeredBy := "local"
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			triggeredBy = "github-actions"
		}

		logEntry := servicekit.BuildLogEntry{
			Service:         serviceName,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			TriggeredBy:     triggeredBy,
			ForceUpdate:     forceFlag,
			TranslationUsed: translateFlag,
			NewAdded:        len(newIndices),
			TotalAfterSync:  len(merged),
			Status:          "success",
		}

		buildLogResult, err := cmaClient.GetBuildLog(ctx)
		if err != nil {
			log.Printf("WARNING: failed to fetch build log: %v", err)
			return nil
		}

		var ownEntries, otherEntries []servicekit.BuildLogEntry
		for _, e := range buildLogResult.Entries {
			if e.Service == serviceName {
				ownEntries = append(ownEntries, e)
			} else {
				otherEntries = append(otherEntries, e)
			}
		}
		if len(ownEntries) >= 3 {
			ownEntries = ownEntries[len(ownEntries)-2:]
		}
		allLogEntries := append(otherEntries, append(ownEntries, logEntry)...)

		var buildLogEntryID string
		var buildLogVersion int

		if buildLogResult.EntryID == "" {
			buildLogEntryID, buildLogVersion, err = cmaClient.CreateBuildLog(ctx, allLogEntries)
			if err != nil {
				log.Printf("WARNING: failed to create build log: %v", err)
				return nil
			}
		} else {
			buildLogEntryID = buildLogResult.EntryID
			buildLogVersion, err = cmaClient.UpdateBuildLog(ctx, buildLogResult, allLogEntries)
			if err != nil {
				log.Printf("WARNING: failed to update build log: %v", err)
				return nil
			}
		}

		if err := cmaClient.PublishEntry(ctx, buildLogEntryID, buildLogVersion); err != nil {
			log.Printf("WARNING: failed to publish build log: %v", err)
			return nil
		}

		log.Printf("Build log updated (%d total entries)", len(allLogEntries))
		return nil
	},
}

func init() {
	scrapeCmd.Flags().StringVar(&profileFlag, "profile", "", "LinkedIn username (e.g. alberthiggs)")
	scrapeCmd.Flags().BoolVar(&translateFlag, "translate", false, "Translate quotes to English using Gemini")
	scrapeCmd.Flags().BoolVar(&forceFlag, "force", false, "Replace all existing testimonials instead of merging")
	rootCmd.AddCommand(scrapeCmd)
}
