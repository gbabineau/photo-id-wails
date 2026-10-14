package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadQuizFixture(t *testing.T) {
	quiz, err := loadQuiz(filepath.Join("testdata", "test_data.json"))
	if err != nil {
		t.Fatalf("load quiz fixture: %v", err)
	}
	if quiz.StartMonth != 2 || quiz.EndMonth != 2 || quiz.Location != "EC" {
		t.Fatalf("unexpected quiz details: %+v", quiz)
	}
	if len(quiz.Species) != 4 || quiz.Species[0].CommonName != "Giant Cowbird" {
		t.Fatalf("unexpected species: %+v", quiz.Species)
	}
	if quiz.Species[1].Frequency == nil || *quiz.Species[1].Frequency != 10 {
		t.Fatalf("expected frequency 10, got %+v", quiz.Species[1].Frequency)
	}
}

func TestQuizNameFromPathRemovesJSONExtension(t *testing.T) {
	name := quizNameFromPath(filepath.Join("testdata", "Day 7-8 with 9 species.json"))
	if name != "Day 7-8 with 9 species" {
		t.Fatalf("unexpected quiz name: %q", name)
	}
}

func TestAddHaveListCSVAnnotatesQuizSpecies(t *testing.T) {
	const haveListCSV = "Row #,Category,Common Name,Scientific Name\n1,species,\"Vaux's Swift\",Chaetura vauxi\n2,species,Marbled Murrelet,Brachyramphus marmoratus\n"
	haveList := make(map[string]struct{})
	if err := addHaveListCSV(strings.NewReader(haveListCSV), haveList); err != nil {
		t.Fatalf("parse have-list CSV: %v", err)
	}
	quiz := &Quiz{Species: []Species{
		{CommonName: "  VAUX'S   SWIFT "},
		{CommonName: "Giant Cowbird"},
	}}
	annotateQuizHaveList(quiz, haveList)
	if !quiz.Species[0].SeenBefore || quiz.Species[1].SeenBefore {
		t.Fatalf("unexpected seen-before annotations: %+v", quiz.Species)
	}
}

func TestLoadQuizReadsUppercaseNotesKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiz.json")
	contents := `{"Notes":"Around the lodge","species":[{"comName":"Giant Cowbird"}]}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write quiz file: %v", err)
	}
	quiz, err := loadQuiz(path)
	if err != nil {
		t.Fatalf("load quiz with uppercase Notes: %v", err)
	}
	if quiz.Notes != "Around the lodge" {
		t.Fatalf("unexpected quiz notes: %q", quiz.Notes)
	}
}

func TestLoadQuizRemovesDuplicateCommonNamesKeepingFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiz.json")
	contents := `{"species":[{"comName":"Common Kestrel","frequency":12,"notes":"first"},{"comName":" common   kestrel ","frequency":99,"notes":"duplicate"},{"comName":"Lesser Kestrel"}]}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write quiz file: %v", err)
	}
	quiz, err := loadQuiz(path)
	if err != nil {
		t.Fatalf("load quiz with duplicate species: %v", err)
	}
	if len(quiz.Species) != 2 {
		t.Fatalf("expected 2 unique species, got %d", len(quiz.Species))
	}
	if quiz.Species[0].CommonName != "Common Kestrel" || quiz.Species[0].Notes != "first" || quiz.Species[0].Frequency == nil || *quiz.Species[0].Frequency != 12 {
		t.Fatalf("first species entry was not retained: %+v", quiz.Species[0])
	}
}

func TestLoadQuizRejectsMalformedOrEmptyQuiz(t *testing.T) {
	for name, contents := range map[string]string{
		"malformed": `{"species":[`,
		"empty":     `{"species":[]}`,
		"unnamed":   `{"species":[{}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "quiz.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write quiz file: %v", err)
			}
			if _, err := loadQuiz(path); err == nil {
				t.Fatal("expected invalid quiz to be rejected")
			}
		})
	}
}

func TestResolveSpeciesCodesFromTaxonomy(t *testing.T) {
	quiz, err := loadQuiz(filepath.Join("testdata", "test_data.json"))
	if err != nil {
		t.Fatalf("load quiz fixture: %v", err)
	}
	taxonomy := []Taxon{
		{CommonName: "Giant Cowbird", ScientificName: "Molothrus oryzivorus", SpeciesCode: "giacow", Category: "species", TaxonOrder: 4},
		{CommonName: "White-rumped Swift", ScientificName: "Apus caffer", SpeciesCode: "whrswi1", Category: "species", TaxonOrder: 1},
		{CommonName: "Ortolan Bunting", ScientificName: "Emberiza hortulana", SpeciesCode: "ortbun1", Category: "species", TaxonOrder: 2},
		{CommonName: "Little Bunting", ScientificName: "Emberiza pusilla", SpeciesCode: "litbun", Category: "species", TaxonOrder: 3},
	}

	if unresolved := resolveSpeciesCodes(quiz, taxonomy); len(unresolved) != 0 {
		t.Fatalf("unexpected unresolved species: %v", unresolved)
	}
	if quiz.Species[0].CommonName != "White-rumped Swift" || quiz.Species[0].SpeciesCode != "whrswi1" {
		t.Fatalf("species were not resolved and taxonomically sorted: %+v", quiz.Species)
	}
}

func TestConfiguredTaxonomyCacheResolvesProvidedQuiz(t *testing.T) {
	if os.Getenv("EBIRD_TAXONOMY_PATH") == "" {
		t.Skip("set EBIRD_TAXONOMY_PATH to verify an external taxonomy cache")
	}
	taxonomy, err := loadTaxonomy(t.Context())
	if err != nil {
		t.Fatalf("load configured taxonomy: %v", err)
	}
	quiz, err := loadQuiz(filepath.Join("testdata", "test_data.json"))
	if err != nil {
		t.Fatalf("load quiz fixture: %v", err)
	}
	if unresolved := resolveSpeciesCodes(quiz, taxonomy); len(unresolved) != 0 {
		t.Fatalf("provided taxonomy did not resolve fixture species: %v", unresolved)
	}
	codes := make(map[string]string, len(quiz.Species))
	for _, species := range quiz.Species {
		codes[species.CommonName] = species.SpeciesCode
	}
	if codes["Giant Cowbird"] != "giacow" || codes["White-rumped Swift"] != "whrswi1" || codes["Ortolan Bunting"] != "ortbun1" || codes["Little Bunting"] != "litbun" {
		t.Fatalf("unexpected species-code mappings: %v", codes)
	}
}

func TestBuildEbirdMediaURL(t *testing.T) {
	t.Run("regional season", func(t *testing.T) {
		parsed, err := url.Parse(buildEbirdMediaURL("giacow", "EC", 2, 2))
		if err != nil {
			t.Fatalf("parse generated catalog URL: %v", err)
		}
		query := parsed.Query()
		if query.Get("taxonCode") != "giacow" || query.Get("regionCode") != "EC" || query.Get("beginMonth") != "2" || query.Get("endMonth") != "2" {
			t.Fatalf("unexpected catalog query: %v", query)
		}
	})
	t.Run("full year omits month filters", func(t *testing.T) {
		parsed, err := url.Parse(buildEbirdMediaURL("giacow", "", 1, 12))
		if err != nil {
			t.Fatalf("parse generated catalog URL: %v", err)
		}
		if parsed.Query().Has("beginMonth") || parsed.Query().Has("regionCode") {
			t.Fatalf("unexpected full-year query: %v", parsed.Query())
		}
	})
}

func TestExtractEbirdImageURLs(t *testing.T) {
	var content strings.Builder
	for index := 0; index < 6; index++ {
		fmt.Fprintf(&content, "https://cdn.download.ams.birds.cornell.edu/api/v1/asset/%d/640 ", index)
	}
	images := extractEbirdImageURLs(content.String())
	if len(images) != 2 || !strings.HasSuffix(images[0], "/2/640") || !strings.HasSuffix(images[1], "/4/640") {
		t.Fatalf("unexpected extracted image URLs: %v", images)
	}
	if images := extractEbirdImageURLs("https://cdn.download.ams.birds.cornell.edu/api/v1/asset/1/640"); len(images) != 0 {
		t.Fatalf("expected too few image results to be rejected: %v", images)
	}
}

func TestMediaCacheExpiresAfterEightMonths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cached-item")
	if err := os.WriteFile(path, []byte("cached"), 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(path, now.Add(-mediaCacheTTL+time.Hour), now.Add(-mediaCacheTTL+time.Hour)); err != nil {
		t.Fatalf("set fresh cache time: %v", err)
	}
	if content, err := readFreshCacheFile(path, now); err != nil || string(content) != "cached" {
		t.Fatalf("expected cache entry within TTL, got %q, %v", content, err)
	}
	if err := os.Chtimes(path, now.Add(-mediaCacheTTL-time.Hour), now.Add(-mediaCacheTTL-time.Hour)); err != nil {
		t.Fatalf("set expired cache time: %v", err)
	}
	if _, err := readFreshCacheFile(path, now); !os.IsNotExist(err) {
		t.Fatalf("expected expired cache entry to be removed, got %v", err)
	}
}

func TestGetPhotoDataURLRejectsUntrustedURLs(t *testing.T) {
	if _, err := getPhotoDataURL("https://example.com/photo.jpg"); err == nil {
		t.Fatal("expected external photo URL to be rejected")
	}
}

func TestLiveEbirdCatalog(t *testing.T) {
	if os.Getenv("PHOTO_ID_LIVE_MEDIA_TEST") != "1" {
		t.Skip("set PHOTO_ID_LIVE_MEDIA_TEST=1 to exercise the live eBird catalog")
	}
	result, err := loadQuizPhotos(Quiz{
		Location:   "EC",
		StartMonth: 2,
		EndMonth:   2,
		Species: []Species{{
			CommonName:  "Giant Cowbird",
			SpeciesCode: "giacow",
		}},
	})
	if err != nil {
		t.Fatalf("load photos through Playwright: %v", err)
	}
	if len(result.Photos["giacow"]) == 0 {
		t.Fatalf("no live eBird photos returned; warnings: %v", result.Warnings)
	}
}
