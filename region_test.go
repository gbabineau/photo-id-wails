package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func useRegionServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	old := eBirdRegionInfoURL
	eBirdRegionInfoURL = server.URL + "/"
	t.Cleanup(func() { eBirdRegionInfoURL = old })
	t.Setenv("EBIRDAPIKEY", "key")
}

func TestGetRegionNameRejectsInvalidCodes(t *testing.T) {
	useTempTaxonomyCache(t)
	for _, code := range []string{"", "  ", "../etc", "US-VA-001-X", "A B", "TOOLONGCODE"} {
		if name := (&App{}).GetRegionName(code); name != "" {
			t.Errorf("code %q: got %q", code, name)
		}
	}
}

func TestGetRegionNameWithoutKeyOrCacheReturnsEmpty(t *testing.T) {
	useTempTaxonomyCache(t)
	if name := getRegionName(t.Context(), "AR"); name != "" {
		t.Fatalf("got %q", name)
	}
}

func TestGetRegionNameUsesCache(t *testing.T) {
	taxonomyPath := useTempTaxonomyCache(t)
	path := filepath.Join(filepath.Dir(taxonomyPath), "regions", "AR.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("Argentina"), 0o600); err != nil {
		t.Fatal(err)
	}
	if name := getRegionName(t.Context(), " ar "); name != "Argentina" {
		t.Fatalf("got %q", name)
	}
}

func TestGetRegionNameFetchesAndCaches(t *testing.T) {
	useTempTaxonomyCache(t)
	calls := 0
	useRegionServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/US-VA-001" || r.URL.Query().Get("regionNameFormat") != "detailednoqual" || r.Header.Get("X-eBirdApiToken") != "key" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"result":"Accomack County, Virginia"}`))
	})

	for range 2 {
		if name := (&App{}).GetRegionName("us-va-001"); name != "Accomack County, Virginia" {
			t.Fatalf("got %q", name)
		}
	}
	if calls != 1 {
		t.Fatalf("expected the second lookup to be cached, server called %d times", calls)
	}
}

func TestGetRegionNameHandlesBadResponses(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"http error": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
		"bad json":   func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`not json`)) },
		"no result":  func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"result":""}`)) },
	} {
		t.Run(name, func(t *testing.T) {
			useTempTaxonomyCache(t)
			useRegionServer(t, handler)
			if got := getRegionName(t.Context(), "AR"); got != "" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
