package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/config"
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/internal/contentful"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List current testimonials in Contentful",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadContentful()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := contentful.NewClient(cfg.SpaceID, cfg.CMAToken)
		result, err := client.GetTestimonials(ctx)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}

		if len(result.Testimonials) == 0 {
			fmt.Println("No testimonials found.")
			return nil
		}

		for i, t := range result.Testimonials {
			fmt.Printf("%d. %s — %s @ %s\n", i+1, t.Name, t.Role, t.Company)
			quote := t.Quote
			if len(quote) > 120 {
				quote = quote[:117] + "..."
			}
			fmt.Printf("   \"%s\"\n\n", quote)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
