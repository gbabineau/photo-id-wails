package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const eBirdTaxonomyURL = "https://api.ebird.org/v2/ref/taxonomy/ebird?fmt=json&locale=en"

type Taxon struct {
	CommonName     string  `json:"comName"`
	ScientificName string  `json:"sciName"`
	SpeciesCode    string  `json:"speciesCode"`
	Category       string  `json:"category"`
	TaxonOrder     float64 `json:"taxonOrder"`
}

func loadTaxonomy(ctx context.Context) ([]Taxon, error) {
	if path := os.Getenv("EBIRD_TAXONOMY_PATH"); path != "" {
		return readTaxonomy(path)
	}

	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find user cache directory: %w", err)
	}
	cachePath := filepath.Join(cacheDirectory, "photo-id", "taxonomy.json")
	cached, readErr := readTaxonomy(cachePath)
	if readErr == nil && taxonomyCacheIsFresh(cachePath, taxonomyNow()) {
		return cached, nil
	}
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return nil, readErr
	}

	apiKey := strings.TrimSpace(os.Getenv("EBIRDAPIKEY"))
	if apiKey == "" {
		if readErr == nil {
			slog.Warn("taxonomy cache is past its refresh date but no API key is set; using it", "envVar", "EBIRDAPIKEY")
			return cached, nil
		}
		return nil, fmt.Errorf("taxonomy cache not found; set EBIRDAPIKEY or EBIRD_TAXONOMY_PATH")
	}

	data, taxonomy, err := downloadTaxonomy(ctx, apiKey, http.DefaultClient, eBirdTaxonomyURL)
	if err != nil {
		if readErr == nil {
			slog.Warn("taxonomy refresh failed; using older cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return nil, fmt.Errorf("create taxonomy cache directory: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return nil, fmt.Errorf("save taxonomy cache: %w", err)
	}
	return taxonomy, nil
}

var taxonomyNow = time.Now

func taxonomyCacheIsFresh(path string, at time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return taxonomyFreshAt(info.ModTime(), at)
}

// taxonomyFreshAt reports whether a taxonomy downloaded at cachedAt is still current at the given time.
// eBird publishes a new taxonomy in late October, so a download is good until Oct 25. A download
// made during the Oct 25-31 window may predate the release and is replaced once, from Nov 1.
func taxonomyFreshAt(cachedAt, at time.Time) bool {
	cutoff := time.Date(at.Year(), time.October, 25, 0, 0, 0, 0, at.Location())
	if at.Before(cutoff) {
		cutoff = cutoff.AddDate(-1, 0, 0)
	}
	if cachedAt.Before(cutoff) {
		return false
	}
	windowEnd := time.Date(cutoff.Year(), time.November, 1, 0, 0, 0, 0, at.Location())
	return !(cachedAt.Before(windowEnd) && !at.Before(windowEnd))
}

func readTaxonomy(path string) ([]Taxon, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read taxonomy cache: %w", err)
	}
	var taxonomy []Taxon
	if err := json.Unmarshal(data, &taxonomy); err != nil {
		return nil, fmt.Errorf("parse taxonomy cache: %w", err)
	}
	return taxonomy, nil
}

func downloadTaxonomy(ctx context.Context, apiKey string, client *http.Client, endpoint string) ([]byte, []Taxon, error) {
	data, err := ebirdGet(ctx, client, apiKey, endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("download eBird taxonomy: %w", err)
	}
	var taxonomy []Taxon
	if err := json.Unmarshal(data, &taxonomy); err != nil {
		return nil, nil, fmt.Errorf("parse eBird taxonomy response: %w", err)
	}
	if len(taxonomy) == 0 {
		return nil, nil, fmt.Errorf("eBird taxonomy response was empty")
	}
	return data, taxonomy, nil
}

func resolveSpeciesCodes(quiz *Quiz, taxonomy []Taxon) []string {
	byName := make(map[string]Taxon, len(taxonomy))
	for _, taxon := range taxonomy {
		if taxon.CommonName == "" || taxon.SpeciesCode == "" {
			continue
		}
		if taxon.Category != "" && taxon.Category != "species" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(taxon.CommonName))
		previous, exists := byName[key]
		if !exists || previous.Category != "species" && taxon.Category == "species" {
			byName[key] = taxon
		}
	}

	var unresolved []string
	for index := range quiz.Species {
		species := &quiz.Species[index]
		if species.SpeciesCode != "" {
			continue
		}
		taxon, found := byName[strings.ToLower(strings.TrimSpace(species.CommonName))]
		if !found {
			unresolved = append(unresolved, species.CommonName)
			continue
		}
		species.SpeciesCode = taxon.SpeciesCode
		if species.ScientificName == "" {
			species.ScientificName = taxon.ScientificName
		}
		species.TaxonOrder = taxon.TaxonOrder
	}

	sort.SliceStable(quiz.Species, func(left, right int) bool {
		leftOrder := quiz.Species[left].TaxonOrder
		rightOrder := quiz.Species[right].TaxonOrder
		if leftOrder == 0 || rightOrder == 0 {
			return leftOrder != 0 && rightOrder == 0
		}
		return leftOrder < rightOrder
	})
	return unresolved
}
