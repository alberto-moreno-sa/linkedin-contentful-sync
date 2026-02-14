package linkedin

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Scrape navigates to the LinkedIn recommendations page for the given username
// and extracts all visible recommendations.
func Scrape(ctx context.Context, username string, liAtCookie string, verbose bool) ([]Recommendation, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	url := fmt.Sprintf("https://www.linkedin.com/in/%s/details/recommendations/", username)

	// Step 1: Set cookie and navigate
	err := chromedp.Run(taskCtx,
		setCookie("li_at", liAtCookie, ".linkedin.com"),
		chromedp.Navigate(url),
		chromedp.WaitVisible(SelectorMainSection, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Step 2: Scroll to load lazy content
	err = chromedp.Run(taskCtx, scrollToBottom())
	if err != nil {
		return nil, fmt.Errorf("scroll: %w", err)
	}

	// Step 3: Expand all "Show more" buttons
	err = chromedp.Run(taskCtx,
		chromedp.Sleep(2*time.Second),
		expandAllShowMore(),
	)
	if err != nil {
		log.Println("WARNING: could not expand all 'Show more' buttons:", err)
	}

	// Step 4: Try CSS selector extraction
	recs, err := extractWithSelectors(taskCtx)
	if err != nil {
		log.Println("WARNING: CSS selector extraction failed:", err)
	}

	// Step 5: Fallback to JavaScript extraction if selectors returned nothing
	if len(recs) == 0 {
		log.Println("CSS selectors returned 0 results, trying JavaScript fallback...")
		recs, err = extractWithJS(taskCtx)
		if err != nil {
			return nil, fmt.Errorf("JS fallback extraction: %w", err)
		}
	}

	// Step 6: Dump page HTML in verbose mode if still no results
	if len(recs) == 0 && verbose {
		var html string
		_ = chromedp.Run(taskCtx, chromedp.OuterHTML("html", &html))
		log.Println("=== PAGE HTML (verbose debug) ===")
		log.Println(html[:min(len(html), 5000)])
		log.Println("=== END HTML ===")
	}

	return recs, nil
}

func setCookie(name, value, domain string) chromedp.ActionFunc {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		expr := cdp.TimeSinceEpoch(time.Now().Add(365 * 24 * time.Hour))
		return network.SetCookie(name, value).
			WithDomain(domain).
			WithPath("/").
			WithHTTPOnly(true).
			WithSecure(true).
			WithExpires(&expr).
			Do(ctx)
	})
}

func scrollToBottom() chromedp.ActionFunc {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		for i := 0; i < 10; i++ {
			if err := chromedp.Evaluate(`window.scrollBy(0, 800)`, nil).Do(ctx); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
		}
		return nil
	})
}

func expandAllShowMore() chromedp.ActionFunc {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var nodes []*cdp.Node
		err := chromedp.Nodes(SelectorShowMore, &nodes,
			chromedp.ByQueryAll, chromedp.AtLeast(0)).Do(ctx)
		if err != nil || len(nodes) == 0 {
			return nil
		}
		for _, node := range nodes {
			_ = chromedp.MouseClickNode(node).Do(ctx)
			time.Sleep(300 * time.Millisecond)
		}
		return nil
	})
}

func extractWithSelectors(ctx context.Context) ([]Recommendation, error) {
	var items []*cdp.Node
	err := chromedp.Nodes(SelectorRecommendationItem, &items,
		chromedp.ByQueryAll, chromedp.AtLeast(0)).Do(ctx)
	if err != nil {
		return nil, err
	}

	var recs []Recommendation
	for _, item := range items {
		rec := extractSingle(ctx, item)
		if rec.Name != "" && rec.Quote != "" {
			recs = append(recs, rec)
		}
	}
	return recs, nil
}

func extractSingle(ctx context.Context, node *cdp.Node) Recommendation {
	var rec Recommendation

	var name string
	if err := chromedp.Text(SelectorName, &name,
		chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
		rec.Name = strings.TrimSpace(name)
	}

	var headline string
	if err := chromedp.Text(SelectorHeadline, &headline,
		chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
		rec.Role, rec.Company = parseHeadline(strings.TrimSpace(headline))
	}

	var quote string
	if err := chromedp.Text(SelectorQuote, &quote,
		chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil {
		rec.Quote = strings.TrimSpace(quote)
	}

	var avatarURL string
	var ok bool
	if err := chromedp.AttributeValue(SelectorAvatar, "src", &avatarURL, &ok,
		chromedp.ByQuery, chromedp.FromNode(node)).Do(ctx); err == nil && ok {
		rec.AvatarURL = avatarURL
	}

	return rec
}

type jsResult struct {
	Name      string `json:"name"`
	Headline  string `json:"headline"`
	Quote     string `json:"quote"`
	AvatarURL string `json:"avatarUrl"`
}

func extractWithJS(ctx context.Context) ([]Recommendation, error) {
	var results []jsResult
	err := chromedp.Evaluate(JSExtractFallback, &results).Do(ctx)
	if err != nil {
		return nil, err
	}

	var recs []Recommendation
	for _, r := range results {
		role, company := parseHeadline(r.Headline)
		recs = append(recs, Recommendation{
			Name:      r.Name,
			Role:      role,
			Company:   company,
			Quote:     r.Quote,
			AvatarURL: r.AvatarURL,
		})
	}
	return recs, nil
}

// parseHeadline splits "Engineering Manager at Google" into role and company.
func parseHeadline(headline string) (role, company string) {
	for _, sep := range []string{" at ", " @ "} {
		lower := strings.ToLower(headline)
		if idx := strings.LastIndex(lower, sep); idx != -1 {
			return strings.TrimSpace(headline[:idx]),
				strings.TrimSpace(headline[idx+len(sep):])
		}
	}

	parts := strings.SplitN(headline, ",", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}

	return strings.TrimSpace(headline), ""
}
