package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const (
	eBirdMediaURL    = "https://media.ebird.org/catalog"
	mediaUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	mediaWaitTimeout = 35_000
	maxMediaRetries  = 2
	maxPhotos        = 12
	mediaCacheTTL    = 8 * 30 * 24 * time.Hour
	photoFetchLimit  = 25 << 20
)

var (
	assetURLPattern       = regexp.MustCompile(`https://cdn\.download\.ams\.birds\.cornell\.edu/api/v\d+/asset/\d+/640`)
	playwrightInstallOnce sync.Once
	playwrightInstallErr  error
)

type PhotoResult struct {
	Photos   map[string][]string `json:"photos"`
	Warnings []string            `json:"warnings"`
}

type PhotoProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Species string `json:"species"`
	Stage   string `json:"stage"`
}

type photoProgressFunc func(PhotoProgress)

func buildEbirdMediaURL(speciesCode, location string, startMonth, endMonth int) string {
	query := url.Values{}
	query.Set("view", "grid")
	query.Set("taxonCode", speciesCode)
	query.Set("sort", "rating_rank_desc")
	query.Set("mediaType", "photo")
	if location != "" {
		query.Set("regionCode", location)
	}
	if startMonth >= 1 && startMonth <= 12 && endMonth >= 1 && endMonth <= 12 && (startMonth != 1 || endMonth != 12) {
		query.Set("beginMonth", fmt.Sprint(startMonth))
		query.Set("endMonth", fmt.Sprint(endMonth))
	}
	return eBirdMediaURL + "?" + query.Encode()
}

func extractEbirdImageURLs(content string) []string {
	matches := assetURLPattern.FindAllString(content, -1)
	if len(matches) <= 4 {
		return nil
	}

	images := make([]string, 0, maxPhotos)
	for index := 2; index < len(matches) && len(images) < maxPhotos; index += 2 {
		images = append(images, matches[index])
	}
	return images
}

func loadQuizPhotos(quiz Quiz) (*PhotoResult, error) {
	return loadQuizPhotosWithProgress(quiz, nil)
}

func loadQuizPhotosWithProgress(quiz Quiz, reportProgress photoProgressFunc) (*PhotoResult, error) {
	result := &PhotoResult{
		Photos:   make(map[string][]string),
		Warnings: make([]string, 0),
	}
	type pendingSpecies struct {
		index   int
		species Species
	}
	var pending []pendingSpecies

	for index, species := range quiz.Species {
		if species.SpeciesCode == "" {
			result.Warnings = append(result.Warnings, species.CommonName+": no eBird species code")
			continue
		}
		if images, cached := findCachedSpeciesImages(species.SpeciesCode, quiz.Location, quiz.StartMonth, quiz.EndMonth); cached {
			result.Photos[species.SpeciesCode] = images
			if len(images) == 0 {
				result.Warnings = append(result.Warnings, species.CommonName+": no catalog photos found")
			}
			if reportProgress != nil {
				reportProgress(PhotoProgress{Current: index + 1, Total: len(quiz.Species), Species: species.CommonName, Stage: "Using cached catalog"})
			}
			continue
		}
		pending = append(pending, pendingSpecies{index: index, species: species})
	}
	if len(pending) == 0 {
		return result, nil
	}
	if reportProgress != nil {
		reportProgress(PhotoProgress{Total: len(quiz.Species), Stage: "Preparing Chromium"})
	}

	playwrightInstallOnce.Do(func() {
		playwrightInstallErr = playwright.Install(&playwright.RunOptions{
			Browsers:         []string{"chromium"},
			OnlyInstallShell: true,
			Verbose:          false,
		})
	})
	if playwrightInstallErr != nil {
		return nil, fmt.Errorf("install Playwright Chromium: %w", playwrightInstallErr)
	}

	playwrightInstance, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("start Playwright: %w", err)
	}
	defer playwrightInstance.Stop()

	browser, err := playwrightInstance.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("launch Chromium: %w", err)
	}
	defer browser.Close()

	browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String(mediaUserAgent),
	})
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	defer browserContext.Close()

	page, err := browserContext.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create catalog page: %w", err)
	}
	page.SetDefaultTimeout(mediaWaitTimeout)

	for _, item := range pending {
		species := item.species
		if reportProgress != nil {
			reportProgress(PhotoProgress{Current: item.index + 1, Total: len(quiz.Species), Species: species.CommonName, Stage: "Searching eBird catalog"})
		}
		images, err := findSpeciesImages(page, species.SpeciesCode, quiz.Location, quiz.StartMonth, quiz.EndMonth)
		if err != nil {
			result.Warnings = append(result.Warnings, species.CommonName+": "+err.Error())
			if reportProgress != nil {
				reportProgress(PhotoProgress{Current: item.index + 1, Total: len(quiz.Species), Species: species.CommonName, Stage: "Catalog search failed"})
			}
			continue
		}
		result.Photos[species.SpeciesCode] = images
		if len(images) == 0 {
			result.Warnings = append(result.Warnings, species.CommonName+": no catalog photos found")
		}
		if reportProgress != nil {
			reportProgress(PhotoProgress{Current: item.index + 1, Total: len(quiz.Species), Species: species.CommonName, Stage: fmt.Sprintf("Found %d photos", len(images))})
		}
	}
	return result, nil
}

func speciesCatalogQueries(speciesCode, location string, startMonth, endMonth int) []string {
	queries := []string{
		buildEbirdMediaURL(speciesCode, location, startMonth, endMonth),
	}
	if location != "" {
		queries = append(queries, buildEbirdMediaURL(speciesCode, "", startMonth, endMonth))
	}
	if startMonth != 1 || endMonth != 12 {
		queries = append(queries, buildEbirdMediaURL(speciesCode, "", 1, 12))
	}
	return queries
}

func findCachedSpeciesImages(speciesCode, location string, startMonth, endMonth int) ([]string, bool) {
	queries := speciesCatalogQueries(speciesCode, location, startMonth, endMonth)
	var bestImages []string
	allCached := true
	for _, query := range queries {
		content, err := readCachedCatalog(query)
		if err != nil {
			allCached = false
			continue
		}
		images := extractEbirdImageURLs(content)
		if len(images) > 2 {
			return images, true
		}
		if len(images) > len(bestImages) {
			bestImages = images
		}
	}
	return bestImages, allCached
}

func findSpeciesImages(page playwright.Page, speciesCode, location string, startMonth, endMonth int) ([]string, error) {
	queries := speciesCatalogQueries(speciesCode, location, startMonth, endMonth)
	var bestImages []string
	for _, query := range queries {
		content, err := loadCatalogHTML(page, query)
		if err != nil {
			return nil, err
		}
		images := extractEbirdImageURLs(content)
		if len(images) > 2 {
			return images, nil
		}
		if len(images) > len(bestImages) {
			bestImages = images
		}
	}
	return bestImages, nil
}

func loadCatalogHTML(page playwright.Page, catalogURL string) (string, error) {
	if content, err := readCachedCatalog(catalogURL); err == nil {
		return content, nil
	}

	var lastError error
	for attempt := 0; attempt < maxMediaRetries; attempt++ {
		_, err := page.Goto(catalogURL, playwright.PageGotoOptions{
			Timeout: playwright.Float(mediaWaitTimeout),
		})
		if err == nil {
			err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State:   playwright.LoadStateNetworkidle,
				Timeout: playwright.Float(mediaWaitTimeout),
			})
		}
		if err == nil {
			results := page.Locator("ol.ResultsGrid")
			err = results.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateAttached,
				Timeout: playwright.Float(mediaWaitTimeout),
			})
			if err == nil {
				content, innerErr := results.InnerHTML()
				if innerErr == nil {
					cacheCatalog(catalogURL, content)
					return content, nil
				}
				err = innerErr
			}
		}
		lastError = err
	}
	return "", fmt.Errorf("eBird catalog did not produce results after %d attempts: %w", maxMediaRetries, lastError)
}

func catalogCachePath(catalogURL string) (string, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(catalogURL))
	return filepath.Join(cacheDirectory, "photo-id", "media", fmt.Sprintf("%x.html", hash)), nil
}

func readCachedCatalog(catalogURL string) (string, error) {
	path, err := catalogCachePath(catalogURL)
	if err != nil {
		return "", err
	}
	content, err := readFreshCacheFile(path, time.Now())
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func cacheCatalog(catalogURL, content string) {
	path, err := catalogCachePath(catalogURL)
	if err != nil {
		return
	}
	_ = writeCacheFile(path, []byte(strings.TrimSpace(content)))
}

func photoCachePath(photoURL string) (string, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(photoURL))
	return filepath.Join(cacheDirectory, "photo-id", "photos", fmt.Sprintf("%x.img", hash)), nil
}

func getPhotoDataURL(photoURL string) (string, error) {
	parsed, err := url.Parse(photoURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "cdn.download.ams.birds.cornell.edu" || !assetURLPattern.MatchString(photoURL) {
		return "", fmt.Errorf("unsupported photo URL")
	}

	path, err := photoCachePath(photoURL)
	if err != nil {
		return "", fmt.Errorf("find photo cache directory: %w", err)
	}
	data, err := readFreshCacheFile(path, time.Now())
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read cached photo: %w", err)
	}
	if os.IsNotExist(err) {
		client := &http.Client{Timeout: 30 * time.Second}
		request, requestErr := http.NewRequest(http.MethodGet, photoURL, nil)
		if requestErr != nil {
			return "", fmt.Errorf("create photo request: %w", requestErr)
		}
		request.Header.Set("User-Agent", mediaUserAgent)
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return "", fmt.Errorf("download photo: %w", requestErr)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("download photo: %s", response.Status)
		}
		data, requestErr = io.ReadAll(io.LimitReader(response.Body, photoFetchLimit+1))
		if requestErr != nil {
			return "", fmt.Errorf("read photo response: %w", requestErr)
		}
		if len(data) > photoFetchLimit {
			return "", fmt.Errorf("photo exceeds %d-byte cache limit", photoFetchLimit)
		}
		if requestErr = writeCacheFile(path, data); requestErr != nil {
			return "", fmt.Errorf("cache photo: %w", requestErr)
		}
	}

	contentType := http.DetectContentType(data)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "image/") {
		return "", fmt.Errorf("photo response is not an image")
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func readFreshCacheFile(path string, now time.Time) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if now.Sub(info.ModTime()) > mediaCacheTTL {
		_ = os.Remove(path)
		return nil, os.ErrNotExist
	}
	return os.ReadFile(path)
}

func writeCacheFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}
