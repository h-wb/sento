package torznab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// An endpoint behind a login proxy redirects API calls to a sign-in page
// on another host; that must read as the cause, not as a bad response.
func TestPingReportsLoginRedirect(t *testing.T) {
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "<html><title>Login</title></html>")
	}))
	defer login.Close()
	guarded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, login.URL, http.StatusFound)
	}))
	defer guarded.Close()
	open := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<caps><server title="ok"/></caps>`)
	}))
	defer open.Close()

	if err := New(open.URL, "k").Ping(context.Background()); err != nil {
		t.Fatalf("open endpoint: %v", err)
	}
	err := New(guarded.URL, "k").Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "login proxy") {
		t.Fatalf("guarded endpoint error = %v", err)
	}
}

// A search restricted to categories sends them as one cat parameter;
// without categories the parameter is absent.
func TestSearchSendsCategories(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Query().Get("q")+"|"+r.URL.Query().Get("cat"))
		_, _ = fmt.Fprint(w, `<rss><channel><item><title>Hunter x Hunter v01</title><guid>g1</guid></item></channel></rss>`)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	if rel, err := c.Search(context.Background(), "hunter x hunter", CategoryBooks); err != nil || len(rel) != 1 {
		t.Fatalf("search = %+v, %v", rel, err)
	}
	if _, err := c.Search(context.Background(), "hunter x hunter"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"hunter x hunter|7000", "hunter x hunter|"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

// Category ids on results and their names from the capabilities feed are
// what lets a release's indexer section stand in for a missing language.
func TestCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = fmt.Fprint(w, `<caps><categories>
				<category id="7000" name="Books"><subcat id="7030" name="Books/Comics"/></category>
				<category id="156719" name="Literature - English-translated"/>
			</categories></caps>`)
			return
		}
		_, _ = fmt.Fprint(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item>
			<title>Hunter x Hunter v38 (2026) (Digital) (JKO)</title><guid>g1</guid>
			<torznab:attr name="category" value="7000"/><torznab:attr name="category" value="156719"/>
		</item></channel></rss>`)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	names, err := c.Categories(context.Background())
	if err != nil || names[7030] != "Books/Comics" || names[156719] != "Literature - English-translated" {
		t.Fatalf("categories = %v, %v", names, err)
	}
	rel, err := c.Search(context.Background(), "hunter x hunter", CategoryBooks)
	if err != nil || len(rel) != 1 || !slices.Equal(rel[0].Categories, []int{7000, 156719}) {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}
