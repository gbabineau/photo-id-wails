package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	taxonomy, err := readTaxonomy(cachePath)
	if err == nil {
		return taxonomy, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	apiKey := strings.TrimSpace(os.Getenv("EBIRDAPIKEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("taxonomy cache not found; set EBIRDAPIKEY or EBIRD_TAXONOMY_PATH")
	}

	data, taxonomy, err := downloadTaxonomy(ctx, apiKey, http.DefaultClient, eBirdTaxonomyURL)
	if err != nil {
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create taxonomy request: %w", err)
	}
	request.Header.Set("X-eBirdApiToken", apiKey)

	response, err := client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("download eBird taxonomy: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("download eBird taxonomy: %s", response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read eBird taxonomy response: %w", err)
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
