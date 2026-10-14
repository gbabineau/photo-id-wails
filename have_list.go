package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func loadHaveListFromCache() map[string]struct{} {
	haveList := make(map[string]struct{})
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return haveList
	}
	return loadHaveListDirectory(filepath.Join(cacheDir, "photo-id", "have_lists"))
}

func loadHaveListDirectory(dir string) map[string]struct{} {
	haveList := make(map[string]struct{})
	files, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return haveList
	}
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Unable to read have-list file %q: %v", filepath.Base(path), err)
			continue
		}
		err = addHaveListCSV(file, haveList)
		closeErr := file.Close()
		if err != nil {
			log.Printf("Unable to parse have-list file %q: %v", filepath.Base(path), err)
		}
		if closeErr != nil {
			log.Printf("Unable to close have-list file %q: %v", filepath.Base(path), closeErr)
		}
	}
	return haveList
}

func addHaveListCSV(source io.Reader, haveList map[string]struct{}) error {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	commonNameColumn := -1
	for index, column := range header {
		column = strings.TrimPrefix(strings.TrimSpace(column), "\uFEFF")
		if strings.EqualFold(column, "Common Name") || strings.EqualFold(column, "comName") {
			commonNameColumn = index
			break
		}
	}
	if commonNameColumn < 0 {
		return fmt.Errorf("CSV has no Common Name column")
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read row: %w", err)
		}
		if commonNameColumn >= len(row) {
			continue
		}
		if name := normalizeSpeciesName(row[commonNameColumn]); name != "" {
			haveList[name] = struct{}{}
		}
	}
}

func annotateQuizHaveList(quiz *Quiz, haveList map[string]struct{}) {
	for index := range quiz.Species {
		_, quiz.Species[index].SeenBefore = haveList[normalizeSpeciesName(quiz.Species[index].CommonName)]
	}
}

func normalizeSpeciesName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}
