package cvf_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/cvf-cli/cvf"
)

// paperListHTML returns a minimal CVF conference page with n fake papers.
func paperListHTML(n int) string {
	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><body><div id="content"><h3>Papers</h3><dl>`)
	for i := 0; i < n; i++ {
		author1 := fmt.Sprintf("Alice Author%d", i)
		author2 := fmt.Sprintf("Bob Builder%d", i)
		title := fmt.Sprintf("Paper Title Number %d", i)
		slug := fmt.Sprintf("Author%d_Paper_Title_CVPR_2024_paper", i)
		fmt.Fprintf(&sb, `
<dt class="ptitle"><br><a href="/content/CVPR2024/html/%s.html">%s</a></dt>
<dd>
<form id="form-alice%d" action="/CVPR2024" method="post" class="authsearch">
<input type="hidden" name="query_author" value="%s">
<a href="#">%s</a>,
</form>
<form id="form-bob%d" action="/CVPR2024" method="post" class="authsearch">
<input type="hidden" name="query_author" value="%s">
<a href="#">%s</a>
</form>
</dd>
<dd>
[<a href="/content/CVPR2024/papers/%s.pdf">pdf</a>]
</dd>`, slug, title, i, author1, author1, i, author2, author2, slug)
	}
	sb.WriteString(`</dl></div></body></html>`)
	return sb.String()
}

// menuHTML returns a minimal /menu page with a few conference entries.
func menuHTML() string {
	return `<!DOCTYPE html><html><body>
<div id="content">
<h3>CVF Sponsored Conferences</h3>
<dl>
<dd>
CVPR 2024, Seattle Washington [<a href="CVPR2024">Main Conference</a>]<br><br>
</dd>
<dd>
ICCV 2023, Paris France [<a href="ICCV2023">Main Conference</a>]<br><br>
</dd>
<dd>
CVPR 2023, Vancouver Canada [<a href="CVPR2023">Main Conference</a>]<br><br>
</dd>
</dl>
</div>
</body></html>`
}

func newTestServer(t *testing.T) (*httptest.Server, *cvf.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		switch {
		case r.URL.Path == "/menu":
			fmt.Fprint(w, menuHTML())
		case strings.HasPrefix(r.URL.Path, "/CVPR2024"):
			fmt.Fprint(w, paperListHTML(5))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := cvf.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return srv, cvf.NewClient(cfg)
}

func TestPapers(t *testing.T) {
	_, client := newTestServer(t)

	papers, err := client.Papers(context.Background(), "CVPR", 2024, 0)
	if err != nil {
		t.Fatalf("Papers: %v", err)
	}
	if len(papers) != 5 {
		t.Errorf("got %d papers, want 5", len(papers))
	}

	p := papers[0]
	if p.Title == "" {
		t.Error("paper has empty title")
	}
	if p.Conference != "CVPR" {
		t.Errorf("conference = %q, want CVPR", p.Conference)
	}
	if p.Year != 2024 {
		t.Errorf("year = %d, want 2024", p.Year)
	}
	if len(p.Authors) == 0 {
		t.Error("paper has no authors")
	}
	if p.PDFURL == "" {
		t.Error("paper has empty PDF URL")
	}
	if p.AbstractURL == "" {
		t.Error("paper has empty abstract URL")
	}
}

func TestPapersLimit(t *testing.T) {
	_, client := newTestServer(t)

	papers, err := client.Papers(context.Background(), "CVPR", 2024, 3)
	if err != nil {
		t.Fatalf("Papers: %v", err)
	}
	if len(papers) != 3 {
		t.Errorf("got %d papers, want 3", len(papers))
	}
}

func TestSearch(t *testing.T) {
	_, client := newTestServer(t)

	// "title number 2" should match "Paper Title Number 2"
	results, err := client.Search(context.Background(), "Number 2", "CVPR", 2024, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
	if len(results) > 0 && !strings.Contains(results[0].Title, "2") {
		t.Errorf("unexpected result title: %q", results[0].Title)
	}
}

func TestSearchByAuthor(t *testing.T) {
	_, client := newTestServer(t)

	// "alice author3" is an author of paper 3
	results, err := client.Search(context.Background(), "alice author3", "CVPR", 2024, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestConferences(t *testing.T) {
	_, client := newTestServer(t)

	confs, err := client.Conferences(context.Background())
	if err != nil {
		t.Fatalf("Conferences: %v", err)
	}
	if len(confs) == 0 {
		t.Fatal("got 0 conferences")
	}

	// first entry should be CVPR 2024
	found := false
	for _, c := range confs {
		if c.Name == "CVPR" && c.Year == 2024 {
			found = true
			if c.Location == "" {
				t.Error("conference has empty location")
			}
			if c.URL == "" {
				t.Error("conference has empty URL")
			}
		}
	}
	if !found {
		t.Errorf("CVPR 2024 not in conferences: %+v", confs)
	}
}

func TestPapersNotFound(t *testing.T) {
	_, client := newTestServer(t)

	// ECCV9999 will return 404 from our test server
	_, err := client.Papers(context.Background(), "ECCV", 9999, 0)
	if err == nil {
		t.Fatal("expected error for unknown conference, got nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := cvf.DefaultConfig()
	if cfg.BaseURL == "" {
		t.Error("DefaultConfig has empty BaseURL")
	}
	if cfg.UserAgent == "" {
		t.Error("DefaultConfig has empty UserAgent")
	}
	if cfg.Retries <= 0 {
		t.Error("DefaultConfig has non-positive Retries")
	}
}
