# linkedin-contentful-sync

CLI tool that scrapes your LinkedIn recommendations and syncs them as testimonials to Contentful CMS. Optionally translates quotes to English using Google Gemini.

## Features

- Scrapes recommendations from your LinkedIn profile via the Voyager API
- Uploads recommender avatars as Contentful assets (CDN-hosted)
- Fetches recommender details: name, role, company, LinkedIn URL
- Translates quotes to English using Google Gemini (`--translate`)
- Deduplicates by name + company to avoid duplicates on re-runs
- Force replace mode to overwrite existing testimonials (`--force`)
- GitHub Actions workflow for manual execution

## Prerequisites

- Go 1.21+
- A Contentful space with a `siteSection` content type containing a `testimonials` JSON field
- A LinkedIn `li_at` session cookie
- (Optional) A Google Gemini API key for translation

## Setup

1. Clone the repo:

```bash
git clone https://github.com/alberto-moreno-sa/linkedin-contentful-sync.git
cd linkedin-contentful-sync
```

2. Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

| Variable | Description |
|---|---|
| `CONTENTFUL_SPACE_ID` | Your Contentful space ID |
| `CONTENTFUL_CMA_TOKEN` | Content Management API token |
| `LINKEDIN_COOKIE` | Value of the `li_at` cookie from linkedin.com |
| `GEMINI_API_KEY` | Google Gemini API key (only needed with `--translate`) |

### Getting the LinkedIn cookie

1. Log in to [linkedin.com](https://www.linkedin.com) in your browser
2. Open DevTools (F12) > Application > Cookies > `linkedin.com`
3. Copy the value of the `li_at` cookie

> **Note:** The `li_at` cookie expires approximately every 2 months.

### Getting the Gemini API key

1. Go to [Google AI Studio](https://aistudio.google.com/apikey)
2. Click "Create API Key"
3. Copy the generated key

## Usage

### Scrape and sync

```bash
go run . scrape --profile=your-linkedin-username
```

### Scrape with translation

```bash
go run . scrape --profile=your-linkedin-username --translate
```

### Force replace all testimonials

```bash
go run . scrape --profile=your-linkedin-username --translate --force
```

### List existing testimonials

```bash
go run . list
```

### Build

```bash
make build
./bin/linkedin-sync scrape --profile=your-linkedin-username
```

## GitHub Actions

The repo includes a manual workflow at `.github/workflows/sync.yml`.

### Setup

1. Go to your repo **Settings > Secrets and variables > Actions**
2. Add these secrets:
   - `CONTENTFUL_SPACE_ID`
   - `CONTENTFUL_CMA_TOKEN`
   - `GEMINI_API_KEY`

### Run

1. Go to **Actions > Sync LinkedIn Recommendations**
2. Click **Run workflow**
3. Paste your `li_at` cookie and select options

## Project structure

```
├── cmd/
│   ├── root.go           # CLI root command
│   ├── scrape.go         # Scrape + sync command
│   └── list.go           # List testimonials command
├── internal/
│   ├── config/           # Environment variable loading
│   ├── contentful/       # Contentful CMA client (CRUD + asset upload)
│   ├── linkedin/         # LinkedIn Voyager API scraper
│   ├── sync/             # Merge/deduplication logic
│   └── translate/        # Google Gemini translation
├── .github/workflows/    # GitHub Actions workflow
├── .env.example          # Environment template
├── Makefile
└── main.go
```

## Disclaimer

This tool uses LinkedIn's undocumented Voyager API. It is intended for personal use to sync your own profile's recommendations. LinkedIn may change or restrict access to these endpoints at any time.

## License

[MIT](LICENSE)
