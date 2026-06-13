// Package eleutherai is the library behind the eai command line:
// the HTTP client, RSS feed parsing, and typed data models for the
// EleutherAI research blog at blog.eleuther.ai.
//
// The Client is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package eleutherai

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to the blog server. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "eai/dev (+https://github.com/tamnd/eleutherai-cli)"

// DefaultBaseURL is the root of the EleutherAI blog.
const DefaultBaseURL = "https://blog.eleuther.ai"

// Config holds the tuneable parameters for a Client.
type Config struct {
	// BaseURL is the blog root, default DefaultBaseURL.
	BaseURL   string
	Rate      time.Duration
	Retries   int
	UserAgent string
}

// DefaultConfig returns a Config with conservative, polite defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   DefaultBaseURL,
		Rate:      200 * time.Millisecond,
		Retries:   5,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to the blog over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client configured by cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Post is a single blog post entry.
type Post struct {
	Rank      int    `json:"rank"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Published string `json:"published"`
	Summary   string `json:"summary"`
	Tags      string `json:"tags"`
	URL       string `json:"url"`
}

// rss2Feed is the RSS 2.0 wire format emitted by Hugo on blog.eleuther.ai.
type rss2Feed struct {
	Channel struct {
		Items []rss2Item `xml:"item"`
	} `xml:"channel"`
}

type rss2Item struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Author      string   `xml:"author"`
	Categories  []string `xml:"category"`
}

// Latest fetches the blog's RSS feed and returns up to limit posts, newest first.
// Pass limit <= 0 to get all posts in the feed.
func (c *Client) Latest(ctx context.Context, limit int) ([]Post, error) {
	url := c.cfg.BaseURL + "/index.xml"
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseFeed(body, limit)
}

// Search filters all posts in the RSS feed whose title, summary, author, or tags
// contain query (case-insensitive). Pass limit <= 0 to return all matches.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Post, error) {
	posts, err := c.Latest(ctx, 0)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []Post
	for _, p := range posts {
		if strings.Contains(strings.ToLower(p.Title), q) ||
			strings.Contains(strings.ToLower(p.Summary), q) ||
			strings.Contains(strings.ToLower(p.Author), q) ||
			strings.Contains(strings.ToLower(p.Tags), q) {
			out = append(out, p)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// parseFeed unmarshals an RSS 2.0 body into Post records.
func parseFeed(body []byte, limit int) ([]Post, error) {
	var feed rss2Feed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	items := feed.Channel.Items
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	posts := make([]Post, 0, len(items))
	for i, it := range items {
		posts = append(posts, Post{
			Rank:      i + 1,
			Title:     cleanText(it.Title),
			Author:    cleanText(it.Author),
			Published: formatDate(it.PubDate),
			Summary:   cleanSummary(it.Description),
			Tags:      strings.Join(it.Categories, ", "),
			URL:       strings.TrimSpace(it.Link),
		})
	}
	return posts, nil
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

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

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
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

var reHTMLTag = regexp.MustCompile(`<[^>]+>`)

// cleanSummary strips HTML tags from an RSS description and collapses whitespace.
func cleanSummary(s string) string {
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:199] + "…"
	}
	return s
}

// cleanText decodes HTML entities and trims space.
func cleanText(s string) string {
	return strings.TrimSpace(html.UnescapeString(s))
}

// formatDate parses the RFC1123Z date Hugo emits in RSS and reformats as YYYY-MM-DD.
func formatDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range []string{
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		time.RFC1123Z,
		time.RFC1123,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}
