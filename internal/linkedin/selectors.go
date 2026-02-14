package linkedin

// LinkedIn recommendations page selectors.
// These WILL break when LinkedIn changes their markup.
// Inspect https://www.linkedin.com/in/<user>/details/recommendations/
// in Chrome DevTools to verify/update.

const (
	// Container for each recommendation card.
	SelectorRecommendationItem = `section.artdeco-card li.pvs-list__paged-list-item`

	// Recommender's name.
	SelectorName = `.text-body-medium-bold span[aria-hidden="true"]`

	// Recommender's headline (role + company), e.g. "Engineering Manager at Google".
	SelectorHeadline = `.text-body-small span[aria-hidden="true"]`

	// Recommendation text body.
	SelectorQuote = `.pvs-list__outer-container .inline-show-more-text span[aria-hidden="true"]`

	// Avatar image.
	SelectorAvatar = `img.evi-image`

	// "Show more" button to expand truncated text.
	SelectorShowMore = `button.inline-show-more-text__button`

	// Main section container — used to confirm page has loaded.
	SelectorMainSection = `main section`
)

// jsExtractFallback is JavaScript evaluated in the page context as a fallback
// when CSS selectors return 0 results.
const JSExtractFallback = `
(() => {
	const items = document.querySelectorAll('li.pvs-list__paged-list-item');
	return Array.from(items).map(item => {
		const nameEl = item.querySelector('.text-body-medium-bold span[aria-hidden="true"]') ||
		               item.querySelector('[data-field="name"]');
		const headlineEl = item.querySelector('.text-body-small span[aria-hidden="true"]');
		const textEl = item.querySelector('.inline-show-more-text span[aria-hidden="true"]') ||
		               item.querySelector('.pvs-list__outer-container span[aria-hidden="true"]');
		const imgEl = item.querySelector('img.evi-image') || item.querySelector('img');
		return {
			name: nameEl?.innerText?.trim() || '',
			headline: headlineEl?.innerText?.trim() || '',
			quote: textEl?.innerText?.trim() || '',
			avatarUrl: imgEl?.src || ''
		};
	}).filter(r => r.name && r.quote);
})()
`
