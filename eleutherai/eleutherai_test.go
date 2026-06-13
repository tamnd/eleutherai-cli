package eleutherai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/eleutherai-cli/eleutherai"
)

func sampleFeed(baseURL string) string {
	return `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>Blog on EleutherAI Blog</title>
    <link>` + baseURL + `/</link>
    <item>
      <title>Early Indicators of Reward Hacking</title>
      <link>` + baseURL + `/reward-hacking-indicators/</link>
      <pubDate>Wed, 15 Apr 2026 00:00:00 +0000</pubDate>
      <author>Alice Researcher</author>
      <category>alignment</category>
      <category>reward-hacking</category>
      <description>Using importance sampling to predict reward hacking emergence.</description>
    </item>
    <item>
      <title>The Common Pile v0.1</title>
      <link>` + baseURL + `/common-pile/</link>
      <pubDate>Thu, 05 Jun 2025 14:00:00 -0600</pubDate>
      <author>Bob Dataset</author>
      <category>datasets</category>
      <description>Announcing the Common Pile: an 8TB dataset of open licensed text.</description>
    </item>
    <item>
      <title>Attention Probes</title>
      <link>` + baseURL + `/attention-probes/</link>
      <pubDate>Fri, 01 Aug 2025 15:00:00 +0000</pubDate>
      <author>Carol Mechanistic</author>
      <category>interpretability</category>
      <description>Adding attention to linear probes for mechanistic interpretability.</description>
    </item>
  </channel>
</rss>`
}

func newTestServer(t *testing.T) (*httptest.Server, *eleutherai.Client) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleFeed(srv.URL)))
	}))
	cfg := eleutherai.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	return srv, eleutherai.NewClient(cfg)
}

func TestLatest(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d posts, want 3", len(posts))
	}
	if posts[0].Title != "Early Indicators of Reward Hacking" {
		t.Errorf("first title = %q", posts[0].Title)
	}
	if posts[0].Rank != 1 {
		t.Errorf("rank = %d, want 1", posts[0].Rank)
	}
	if posts[0].Published != "2026-04-15" {
		t.Errorf("published = %q, want 2026-04-15", posts[0].Published)
	}
	if !strings.HasPrefix(posts[0].URL, srv.URL) {
		t.Errorf("unexpected URL %q", posts[0].URL)
	}
	if posts[0].Author != "Alice Researcher" {
		t.Errorf("author = %q, want Alice Researcher", posts[0].Author)
	}
	if !strings.Contains(posts[0].Tags, "alignment") {
		t.Errorf("tags = %q, want to contain 'alignment'", posts[0].Tags)
	}
}

func TestLatestLimit(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Latest(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
}

func TestSearch(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Search(context.Background(), "attention", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d results for 'attention', want 1", len(posts))
	}
	if posts[0].Title != "Attention Probes" {
		t.Errorf("title = %q", posts[0].Title)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Search(context.Background(), "DATASET", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d results for 'DATASET', want 1", len(posts))
	}
}

func TestSearchByAuthor(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Search(context.Background(), "mechanistic", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d results for 'mechanistic', want 1", len(posts))
	}
}

func TestSearchNoMatch(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Search(context.Background(), "kubernetes", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d results, want 0", len(posts))
	}
}

func TestClientRetries(t *testing.T) {
	var hits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleFeed(srv.URL)))
	}))
	defer srv.Close()

	cfg := eleutherai.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := eleutherai.NewClient(cfg)

	posts, err := c.Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) == 0 {
		t.Error("got no posts after retries")
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
}
