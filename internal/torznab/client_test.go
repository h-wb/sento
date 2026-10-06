package torznab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
