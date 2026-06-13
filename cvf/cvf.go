// Package cvf is the library behind the cvf command: the HTTP client,
// request shaping, and the typed data models for Computer Vision Foundation
// open access papers at https://openaccess.thecvf.com/
//
// The site lists papers from conferences like CVPR, ICCV, ECCV, and WACV.
// Pages use plain HTML with no JavaScript required for the paper listing,
// so all parsing uses stdlib strings — no external HTML parser needed.
package cvf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to the CVF site.
const DefaultUserAgent = "cvf/dev (+https://github.com/tamnd/cvf-cli)"

// DefaultBaseURL is the CVF open access root.
const DefaultBaseURL = "https://openaccess.thecvf.com"

// ErrNotFound is returned when a conference or year produces no papers.
var ErrNotFound = fmt.Errorf("not found")

// Config holds constructor parameters for a Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   DefaultBaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the CVF open access site.
type Client struct {
	baseURL   string
	http      *http.Client
	userAgent string
	rate      time.Duration
	retries   int
	last      time.Time
}

// NewClient returns a Client built from cfg.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		http:      &http.Client{Timeout: cfg.Timeout},
		userAgent: cfg.UserAgent,
		rate:      cfg.Rate,
		retries:   cfg.Retries,
	}
}

// Paper represents a single CVF open access paper.
type Paper struct {
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Conference  string   `json:"conference"`
	Year        int      `json:"year"`
	PDFURL      string   `json:"pdf_url"`
	AbstractURL string   `json:"abstract_url"`
}

// Conference represents a conference entry from the menu.
type Conference struct {
	Name     string `json:"name"`
	Year     int    `json:"year"`
	Location string `json:"location"`
	URL      string `json:"url"`
}

// Papers fetches papers from the given conference and year. Pass limit=0 for all.
// Defaults: conference="CVPR", year=2024.
func (c *Client) Papers(ctx context.Context, conference string, year int, limit int) ([]Paper, error) {
	if conference == "" {
		conference = "CVPR"
	}
	if year == 0 {
		year = 2024
	}
	key := fmt.Sprintf("%s%d", strings.ToUpper(conference), year)
	rawURL := fmt.Sprintf("%s/%s?day=all", c.baseURL, key)

	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	papers := parsePapers(string(body), strings.ToUpper(conference), year, c.baseURL)
	if len(papers) == 0 {
		return nil, ErrNotFound
	}
	if limit > 0 && limit < len(papers) {
		papers = papers[:limit]
	}
	return papers, nil
}

// Search returns papers whose title or authors contain query (case-insensitive).
func (c *Client) Search(ctx context.Context, query, conference string, year int, limit int) ([]Paper, error) {
	all, err := c.Papers(ctx, conference, year, 0)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []Paper
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Title), q) {
			out = append(out, p)
		} else {
			for _, a := range p.Authors {
				if strings.Contains(strings.ToLower(a), q) {
					out = append(out, p)
					break
				}
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Conferences fetches the list of available conferences from the menu page.
func (c *Client) Conferences(ctx context.Context) ([]Conference, error) {
	rawURL := c.baseURL + "/menu"
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return parseConferences(string(body), c.baseURL), nil
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ─── Parsing ──────────────────────────────────────────────────────────────────

// parsePapers extracts Paper records from the CVF conference HTML page.
// The page uses a <dl> list where:
//   - <dt class="ptitle"> contains the paper title link
//   - <dd> elements contain author forms and the PDF link
//
// We parse using stdlib strings only — no HTML parser.
func parsePapers(html, conference string, year int, baseURL string) []Paper {
	var papers []Paper

	// Split on paper title markers.
	// Each paper starts with <dt class="ptitle">
	parts := strings.Split(html, `<dt class="ptitle">`)
	for i, part := range parts {
		if i == 0 {
			continue // header before first paper
		}

		// Extract title and abstract URL from the <a href="...">Title</a> inside <dt>
		title, abstractURL := extractTitleAndURL(part, baseURL)
		if title == "" {
			continue
		}

		// Everything up to the next <dt> is this paper's <dd> content.
		// The authors are in <input type="hidden" name="query_author" value="...">
		authors := extractAuthors(part)

		// PDF link: [<a href="/content/...pdf">pdf</a>]
		pdfURL := extractPDFURL(part, baseURL)

		papers = append(papers, Paper{
			Title:       title,
			Authors:     authors,
			Conference:  conference,
			Year:        year,
			PDFURL:      pdfURL,
			AbstractURL: abstractURL,
		})
	}
	return papers
}

// extractTitleAndURL pulls the title text and href from the first anchor in s.
func extractTitleAndURL(s, baseURL string) (title, url string) {
	// Find <a href="...">Title</a>
	aStart := strings.Index(s, "<a href=\"")
	if aStart < 0 {
		return "", ""
	}
	rest := s[aStart+len("<a href=\""):]
	hrefEnd := strings.Index(rest, "\"")
	if hrefEnd < 0 {
		return "", ""
	}
	href := rest[:hrefEnd]

	// Text between > and </a>
	textStart := strings.Index(rest, ">")
	if textStart < 0 {
		return "", ""
	}
	textEnd := strings.Index(rest[textStart:], "</a>")
	if textEnd < 0 {
		return "", ""
	}
	title = strings.TrimSpace(rest[textStart+1 : textStart+textEnd])
	title = htmlUnescape(title)

	if strings.HasPrefix(href, "http") {
		url = href
	} else {
		url = baseURL + href
	}
	return title, url
}

// extractAuthors collects all query_author hidden input values.
func extractAuthors(s string) []string {
	var authors []string
	needle := `name="query_author" value="`
	rest := s
	for {
		idx := strings.Index(rest, needle)
		if idx < 0 {
			break
		}
		rest = rest[idx+len(needle):]
		end := strings.Index(rest, "\"")
		if end < 0 {
			break
		}
		name := htmlUnescape(strings.TrimSpace(rest[:end]))
		if name != "" {
			authors = append(authors, name)
		}
		rest = rest[end:]
	}
	return authors
}

// extractPDFURL finds the [pdf] link in the <dd> block.
func extractPDFURL(s, baseURL string) string {
	// Look for >pdf</a>
	idx := strings.Index(s, ">pdf</a>")
	if idx < 0 {
		return ""
	}
	// Walk back to find href="
	before := s[:idx]
	hrefIdx := strings.LastIndex(before, `href="`)
	if hrefIdx < 0 {
		return ""
	}
	rest := before[hrefIdx+len(`href="`):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	href := rest[:end]
	if strings.HasPrefix(href, "http") {
		return href
	}
	return baseURL + href
}

// parseConferences extracts Conference records from the /menu page.
// Each entry looks like:
//
//	CVPR 2024, Seattle Washington [<a href="CVPR2024">Main Conference</a>] ...
func parseConferences(html, baseURL string) []Conference {
	var confs []Conference

	// Focus on the content div
	start := strings.Index(html, `<div id="content">`)
	if start >= 0 {
		html = html[start:]
	}

	// Each <dd> element is one conference entry
	parts := strings.Split(html, "<dd>")
	for i, part := range parts {
		if i == 0 {
			continue
		}
		// Stop at footer / non-conference content
		if strings.Contains(part, `<div id="footer">`) {
			break
		}
		// Must contain a "Main Conference" link to be a conference entry
		if !strings.Contains(part, "Main Conference") {
			continue
		}

		conf := parseConferenceEntry(part, baseURL)
		if conf.Name != "" {
			confs = append(confs, conf)
		}
	}
	return confs
}

// parseConferenceEntry parses a single <dd> block into a Conference.
func parseConferenceEntry(s, baseURL string) Conference {
	// Text before the first [, e.g. "CVPR 2024, Seattle Washington "
	bracketIdx := strings.Index(s, "[")
	if bracketIdx < 0 {
		return Conference{}
	}
	header := strings.TrimSpace(s[:bracketIdx])
	// Strip any leading <br> or tags
	if idx := strings.LastIndex(header, ">"); idx >= 0 {
		header = strings.TrimSpace(header[idx+1:])
	}

	// Parse "CVPR 2024, Seattle Washington" or "CVPR 2024"
	var name, location string
	var year int
	commaIdx := strings.Index(header, ",")
	if commaIdx >= 0 {
		location = strings.TrimSpace(header[commaIdx+1:])
		header = header[:commaIdx]
	}

	// header is now "CVPR 2024"
	fields := strings.Fields(header)
	if len(fields) >= 2 {
		name = fields[0]
		fmt.Sscanf(fields[1], "%d", &year)
	} else if len(fields) == 1 {
		name = fields[0]
	}

	// Extract the Main Conference URL
	mcIdx := strings.Index(s, "Main Conference")
	if mcIdx < 0 {
		return Conference{}
	}
	hrefBefore := s[:mcIdx]
	hrefIdx := strings.LastIndex(hrefBefore, `href="`)
	var confURL string
	if hrefIdx >= 0 {
		rest := hrefBefore[hrefIdx+len(`href="`):]
		end := strings.Index(rest, "\"")
		if end >= 0 {
			href := rest[:end]
			if strings.HasPrefix(href, "http") {
				confURL = href
			} else {
				confURL = baseURL + "/" + strings.TrimLeft(href, "/")
			}
		}
	}

	if name == "" {
		return Conference{}
	}
	return Conference{
		Name:     name,
		Year:     year,
		Location: location,
		URL:      confURL,
	}
}

// htmlUnescape replaces common HTML entities with their UTF-8 equivalents.
func htmlUnescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
