package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

const (
	voyagerBaseURL    = "https://www.linkedin.com/voyager/api"
	userAgent         = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	profileDecoration = "com.linkedin.voyager.dash.deco.identity.profile.TopCardSupplementary-166"
)

// voyagerClient wraps the HTTP client and CSRF token for Voyager API calls.
type voyagerClient struct {
	httpClient *http.Client
	liAtCookie string
	csrfToken  string
}

func (vc *voyagerClient) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("csrf-token", vc.csrfToken)
	req.Header.Set("x-restli-protocol-version", "2.0.0")
	req.AddCookie(&http.Cookie{Name: "li_at", Value: vc.liAtCookie})
	req.AddCookie(&http.Cookie{Name: "JSESSIONID", Value: vc.csrfToken})
	return req, nil
}

// Scrape fetches LinkedIn recommendations for the given profile using the Voyager API.
func Scrape(ctx context.Context, username string, liAtCookie string, verbose bool) ([]Recommendation, error) {
	client := &http.Client{}

	// Step 1: Get JSESSIONID (CSRF token) by visiting LinkedIn
	csrfToken, err := fetchCSRFToken(ctx, client, liAtCookie)
	if err != nil {
		return nil, fmt.Errorf("csrf token: %w", err)
	}

	vc := &voyagerClient{
		httpClient: client,
		liAtCookie: liAtCookie,
		csrfToken:  csrfToken,
	}

	// Step 2: Resolve profile URN via /me
	profileURN, err := vc.fetchProfileURN(ctx)
	if err != nil {
		return nil, fmt.Errorf("profile URN: %w", err)
	}
	log.Printf("Resolved profile URN: %s", profileURN)

	// Step 3: Fetch recommendations via dash API
	encodedURN := url.QueryEscape(profileURN)
	endpoint := fmt.Sprintf("%s/identity/dash/recommendations?q=received&profileUrn=%s&recommendationStatuses=List(VISIBLE)",
		voyagerBaseURL, encodedURN)

	req, err := vc.newRequest(ctx, "GET", endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyager request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyager API returned %d: %s", resp.StatusCode, string(body))
	}

	var result dashRecommendationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode voyager response: %w", err)
	}

	// Step 4: Enrich each recommendation with recommender profile data
	var recs []Recommendation
	for _, elem := range result.Elements {
		if elem.RecommendationText == "" {
			continue
		}

		rec := Recommendation{
			Quote: elem.RecommendationText,
		}

		// Fetch recommender's profile details
		if elem.RecommenderProfileURN != "" {
			profile, err := vc.fetchProfile(ctx, elem.RecommenderProfileURN)
			if err != nil {
				log.Printf("WARNING: could not fetch profile for recommender: %v", err)
			} else {
				rec.Name = strings.TrimSpace(profile.FirstName + " " + profile.LastName)
				rec.Role = profile.Headline
				if profile.PublicIdentifier != "" {
					rec.LinkedInURL = "https://www.linkedin.com/in/" + profile.PublicIdentifier
				}
				rec.AvatarURL = extractAvatarURL(profile.ProfilePicture)
			}

			// Fetch company separately (requires decoration)
			company, err := vc.fetchCompanyByURN(ctx, elem.RecommenderProfileURN)
			if err != nil {
				log.Printf("WARNING: could not fetch company for %s: %v", rec.Name, err)
			} else {
				rec.Company = company
			}
		}

		if rec.Name != "" && rec.Quote != "" {
			recs = append(recs, rec)
		}
	}

	return recs, nil
}

// fetchProfileURN calls /me to get the logged-in user's profile URN.
func (vc *voyagerClient) fetchProfileURN(ctx context.Context) (string, error) {
	req, err := vc.newRequest(ctx, "GET", voyagerBaseURL+"/me")
	if err != nil {
		return "", err
	}

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch /me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("/me returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		MiniProfile struct {
			DashEntityURN string `json:"dashEntityUrn"`
		} `json:"miniProfile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode /me: %w", err)
	}

	if result.MiniProfile.DashEntityURN == "" {
		return "", fmt.Errorf("dashEntityUrn not found in /me response")
	}

	return result.MiniProfile.DashEntityURN, nil
}

// fetchProfile fetches a profile by URN (no decoration) to get basic info.
func (vc *voyagerClient) fetchProfile(ctx context.Context, profileURN string) (*dashProfile, error) {
	encodedURN := url.PathEscape(profileURN)
	endpoint := fmt.Sprintf("%s/identity/dash/profiles/%s", voyagerBaseURL, encodedURN)

	req, err := vc.newRequest(ctx, "GET", endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("profile API returned %d", resp.StatusCode)
	}

	var profile dashProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode profile: %w", err)
	}

	return &profile, nil
}

// fetchCompanyByURN fetches company name from a profile URN using TopCardSupplementary decoration.
func (vc *voyagerClient) fetchCompanyByURN(ctx context.Context, profileURN string) (string, error) {
	encodedURN := url.PathEscape(profileURN)
	endpoint := fmt.Sprintf("%s/identity/dash/profiles/%s?decorationId=%s",
		voyagerBaseURL, encodedURN, profileDecoration)

	req, err := vc.newRequest(ctx, "GET", endpoint)
	if err != nil {
		return "", err
	}

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch decorated profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("decorated profile API returned %d", resp.StatusCode)
	}

	var result struct {
		ProfileTopPosition struct {
			Elements []struct {
				CompanyName string `json:"companyName"`
			} `json:"elements"`
		} `json:"profileTopPosition"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode decorated profile: %w", err)
	}

	positions := result.ProfileTopPosition.Elements
	if len(positions) > 0 && positions[0].CompanyName != "" {
		return positions[0].CompanyName, nil
	}

	return "", nil
}

// extractAvatarURL picks the best avatar URL from a dashProfile's ProfilePicture.
func extractAvatarURL(pic *dashProfilePicture) string {
	if pic == nil || pic.DisplayImage == nil || pic.DisplayImage.VectorImage == nil {
		return ""
	}
	vi := pic.DisplayImage.VectorImage
	if vi.RootURL == "" || len(vi.Artifacts) == 0 {
		return ""
	}

	bestPath := ""
	for _, a := range vi.Artifacts {
		if bestPath == "" {
			bestPath = a.FileIdentifyingURLPathSegment
		}
		if a.Width == 200 {
			bestPath = a.FileIdentifyingURLPathSegment
			break
		}
	}
	if bestPath == "" {
		return ""
	}
	return vi.RootURL + bestPath
}

// fetchCSRFToken makes a GET to linkedin.com to obtain the JSESSIONID cookie.
// Uses a cookie jar to accumulate cookies across redirects.
func fetchCSRFToken(ctx context.Context, _ *http.Client, liAtCookie string) (string, error) {
	jar, _ := cookiejar.New(nil)
	jarClient := &http.Client{Jar: jar}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.linkedin.com/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.AddCookie(&http.Cookie{Name: "li_at", Value: liAtCookie})

	resp, err := jarClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch linkedin.com: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Check cookies accumulated in the jar across all redirects
	for _, cookie := range jar.Cookies(req.URL) {
		if cookie.Name == "JSESSIONID" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("JSESSIONID cookie not found — li_at cookie may be expired")
}

// --- Response types for the dash API ---

type dashRecommendationsResponse struct {
	Elements []dashRecommendation `json:"elements"`
}

type dashRecommendation struct {
	RecommendationText   string `json:"recommendationText"`
	RecommenderProfileURN string `json:"recommenderProfileUrn"`
}

type dashProfile struct {
	FirstName        string               `json:"firstName"`
	LastName         string               `json:"lastName"`
	Headline         string               `json:"headline"`
	PublicIdentifier string               `json:"publicIdentifier"`
	ProfilePicture   *dashProfilePicture  `json:"profilePicture"`
}

type dashProfilePicture struct {
	DisplayImage *dashDisplayImage `json:"displayImage"`
}

type dashDisplayImage struct {
	VectorImage *dashVectorImage `json:"vectorImage"`
}

type dashVectorImage struct {
	RootURL   string         `json:"rootUrl"`
	Artifacts []dashArtifact `json:"artifacts"`
}

type dashArtifact struct {
	Width                        int    `json:"width"`
	FileIdentifyingURLPathSegment string `json:"fileIdentifyingUrlPathSegment"`
}
