package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx      context.Context
	haveList map[string]struct{}
}

type Quiz struct {
	Name            string    `json:"name"`
	Path            string    `json:"path"`
	StartMonth      int       `json:"start_month"`
	EndMonth        int       `json:"end_month"`
	Location        string    `json:"location"`
	Notes           string    `json:"notes"`
	Species         []Species `json:"species"`
	TaxonomyWarning string    `json:"taxonomyWarning,omitempty"`
}

type Species struct {
	CommonName     string   `json:"comName"`
	ScientificName string   `json:"sciName,omitempty"`
	SpeciesCode    string   `json:"speciesCode,omitempty"`
	Frequency      *float64 `json:"frequency,omitempty"`
	Notes          string   `json:"notes,omitempty"`
	Habitat        string   `json:"Habitat,omitempty"`
	TaxonOrder     float64  `json:"taxonOrder,omitempty"`
	SeenBefore     bool     `json:"seenBefore,omitempty"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.haveList = loadHaveListFromCache()
}

func (a *App) OpenQuiz() (*Quiz, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open Photo ID quiz",
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Quiz files (*.json)",
			Pattern:     "*.json",
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("open quiz file: %w", err)
	}
	if path == "" {
		return nil, nil
	}
	return a.openQuizPath(path)
}

func (a *App) OpenRecentQuiz(path string) (*Quiz, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("quiz path is empty")
	}
	return a.openQuizPath(path)
}

func (a *App) openQuizPath(path string) (*Quiz, error) {
	quiz, err := loadQuiz(path)
	if err != nil {
		return nil, err
	}
	quiz.Name = quizNameFromPath(path)
	quiz.Path = path
	annotateQuizHaveList(quiz, a.haveList)

	taxonomy, err := loadTaxonomy(a.ctx)
	if err != nil {
		quiz.TaxonomyWarning = err.Error()
		return quiz, nil
	}
	if unresolved := resolveSpeciesCodes(quiz, taxonomy); len(unresolved) > 0 {
		quiz.TaxonomyWarning = fmt.Sprintf(
			"No eBird taxonomy match for: %s",
			strings.Join(unresolved, ", "),
		)
	}
	return quiz, nil
}

func quizNameFromPath(path string) string {
	filename := filepath.Base(path)
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

func (a *App) LoadQuizPhotos(quiz Quiz) (*PhotoResult, error) {
	var missingCodes []string
	for _, species := range quiz.Species {
		if species.SpeciesCode == "" {
			missingCodes = append(missingCodes, species.CommonName)
		}
	}
	if len(missingCodes) > 0 {
		if quiz.TaxonomyWarning != "" {
			return nil, fmt.Errorf("resolve species codes before finding photos: %s", quiz.TaxonomyWarning)
		}
		return nil, fmt.Errorf("quiz is missing eBird species codes for: %s; reopen it after configuring the eBird taxonomy", strings.Join(missingCodes, ", "))
	}
	return loadQuizPhotosWithProgress(quiz, func(progress PhotoProgress) {
		wailsruntime.EventsEmit(a.ctx, "photo-search-progress", progress)
	})
}

func (a *App) GetPhotoDataURL(photoURL string) (string, error) {
	return getPhotoDataURL(photoURL)
}

func (a *App) GetDestinationImages(place string) ([]DestinationImage, error) {
	return getDestinationImages(place)
}

func loadQuiz(path string) (*Quiz, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read quiz file: %w", err)
	}

	var quiz Quiz
	if err := json.Unmarshal(data, &quiz); err != nil {
		return nil, fmt.Errorf("parse quiz JSON: %w", err)
	}
	if len(quiz.Species) == 0 {
		return nil, fmt.Errorf("quiz has no species")
	}
	seenSpecies := make(map[string]struct{}, len(quiz.Species))
	uniqueSpecies := quiz.Species[:0]
	for index, species := range quiz.Species {
		if strings.TrimSpace(species.CommonName) == "" {
			return nil, fmt.Errorf("species %d is missing comName", index+1)
		}
		name := normalizeSpeciesName(species.CommonName)
		if _, found := seenSpecies[name]; found {
			continue
		}
		seenSpecies[name] = struct{}{}
		uniqueSpecies = append(uniqueSpecies, species)
	}
	quiz.Species = uniqueSpecies

	return &quiz, nil
}
