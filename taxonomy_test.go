package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func recordEbirdSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	old := ebirdSleep
	ebirdSleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	t.Cleanup(func() { ebirdSleep = old })
	return &waits
}

func TestEbirdGetRetriesAfterRetryAfterSeconds(t *testing.T) {
	waits := recordEbirdSleeps(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-eBirdApiToken") != "key" {
			t.Error("missing token")
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	data, err := ebirdGet(context.Background(), server.Client(), "key", server.URL)
	if err != nil || string(data) != "ok" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Fatalf("waits = %v", *waits)
	}
}

func TestEbirdGetBacksOffWithoutHeaderThenGivesUp(t *testing.T) {
	waits := recordEbirdSleeps(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := ebirdGet(context.Background(), server.Client(), "key", server.URL)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
	if len(*waits) != ebirdMaxAttempts-1 || (*waits)[0] != 2*time.Second || (*waits)[1] != 4*time.Second {
		t.Fatalf("waits = %v", *waits)
	}
}

func TestEbirdGetFailsWhenServerAsksForTooLongAWait(t *testing.T) {
	waits := recordEbirdSleeps(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "86400")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	if _, err := ebirdGet(context.Background(), server.Client(), "key", server.URL); err == nil {
		t.Fatal("expected error")
	}
	if len(*waits) != 0 {
		t.Fatalf("should not have waited: %v", *waits)
	}
}

func TestEbirdGetDoesNotRetryOtherErrors(t *testing.T) {
	waits := recordEbirdSleeps(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	if _, err := ebirdGet(context.Background(), server.Client(), "key", server.URL); err == nil {
		t.Fatal("expected error")
	}
	if len(*waits) != 0 {
		t.Fatalf("waits = %v", *waits)
	}
}

func TestEbirdRetryDelayHTTPDate(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	d, _ := ebirdRetryDelay(now.Add(30*time.Second).Format(http.TimeFormat), 1, now)
	if d != 30*time.Second {
		t.Fatalf("d = %v", d)
	}
}

func TestDownloadTaxonomySendsTokenAndParses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-eBirdApiToken") != "secret" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`[{"comName":"Dipper"}]`))
	}))
	defer server.Close()

	_, taxa, err := downloadTaxonomy(context.Background(), "secret", server.Client(), server.URL)
	if err != nil || len(taxa) != 1 {
		t.Fatalf("taxa=%+v err=%v", taxa, err)
	}
	if _, _, err := downloadTaxonomy(context.Background(), "wrong", server.Client(), server.URL); err == nil {
		t.Fatal("expected error for bad key")
	}
}

func TestTaxonomyFreshAt(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 12, 0, 0, 0, time.UTC) }
	cases := []struct {
		name     string
		cachedAt time.Time
		at       time.Time
		want     bool
	}{
		{"downloaded last November, now mid-October", d(2025, 11, 5), d(2026, 10, 15), true},
		{"downloaded this January", d(2026, 1, 10), d(2026, 10, 24), true},
		{"downloaded before last release window", d(2025, 10, 10), d(2026, 10, 15), false},
		{"downloaded in last release window, now November", d(2025, 10, 28), d(2026, 1, 5), false},
		{"downloaded in release window, still in window", d(2026, 10, 27), d(2026, 10, 30), true},
		{"old cache after release date", d(2025, 11, 5), d(2026, 10, 26), false},
		{"downloaded in release window, now November", d(2026, 10, 27), d(2026, 11, 2), false},
		{"downloaded after window, now later", d(2026, 11, 2), d(2027, 3, 1), true},
	}
	for _, c := range cases {
		if got := taxonomyFreshAt(c.cachedAt, c.at); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func useTempTaxonomyCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("EBIRD_TAXONOMY_PATH", "")
	t.Setenv("EBIRDAPIKEY", "")
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cacheDir, "photo-id", "taxonomy.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadTaxonomyUsesFreshCacheWithoutNetwork(t *testing.T) {
	path := useTempTaxonomyCache(t)
	if err := os.WriteFile(path, []byte(`[{"comName":"Dipper"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	old := taxonomyNow
	taxonomyNow = func() time.Time { return time.Now().AddDate(0, 0, 1) }
	defer func() { taxonomyNow = old }()
	if !taxonomyFreshAt(time.Now(), taxonomyNow()) {
		t.Skip("test clock falls in the October release window")
	}
	taxa, err := loadTaxonomy(context.Background())
	if err != nil || len(taxa) != 1 {
		t.Fatalf("taxa=%v err=%v", taxa, err)
	}
}

func TestLoadTaxonomyUsesStaleCacheWithoutKey(t *testing.T) {
	path := useTempTaxonomyCache(t)
	if err := os.WriteFile(path, []byte(`[{"comName":"Dipper"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := taxonomyNow
	taxonomyNow = func() time.Time { return time.Now().AddDate(5, 0, 0) }
	defer func() { taxonomyNow = old }()

	taxa, err := loadTaxonomy(context.Background())
	if err != nil || len(taxa) != 1 {
		t.Fatalf("taxa=%v err=%v", taxa, err)
	}
}

func TestLoadTaxonomyWithoutKeyOrCacheExplainsConfiguration(t *testing.T) {
	useTempTaxonomyCache(t)
	_, err := loadTaxonomy(context.Background())
	if err == nil || !strings.Contains(err.Error(), "EBIRDAPIKEY") {
		t.Fatalf("err = %v", err)
	}
}
