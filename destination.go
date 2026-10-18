package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	wikipediaAPIURL      = "https://en.wikipedia.org/w/api.php"
	wikimediaImageHost   = "upload.wikimedia.org"
	wikimediaThumbHost   = "thumb.wikimedia.org"
	destinationUserAgent = "photo-id/1.0 (desktop bird photo quiz app)"
	maxDestinationImages = 6
	maxDestinationPlace  = 100
)

var excludedDestinationImage = regexp.MustCompile(`(?i)flag|map|coat[_ ]of|seal|logo|locator|location|symbol|icon|diagram|chart|emblem|portrait|signature`)

type wikiImageListResponse struct {
	Parse struct {
		Title  string   `json:"title"`
		Images []string `json:"images"`
	} `json:"parse"`
}

type wikiImagesResponse struct {
	Query struct {
		Pages map[string]struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				ThumbURL    string          `json:"thumburl"`
				Mime        string          `json:"mime"`
				Width       int             `json:"width"`
				Height      int             `json:"height"`
				ExtMetadata json.RawMessage `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

// imageDescription reads ImageDescription; Wikipedia sends [] instead of {} when metadata is absent.
func imageDescription(raw json.RawMessage) string {
	var metadata map[string]struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return ""
	}
	return metadata["ImageDescription"].Value
}

type DestinationImage struct {
	DataURL string `json:"dataUrl"`
	Caption string `json:"caption"`
	PageURL string `json:"pageUrl"`
}

type destinationImageRef struct {
	ThumbURL string `json:"thumbUrl"`
	Caption  string `json:"caption"`
	PageURL  string `json:"pageUrl"`
}

var (
	htmlTagPattern    = regexp.MustCompile(`<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
	fileExtPattern    = regexp.MustCompile(`(?i)\.jpe?g$`)
)

func cleanCaption(description, title string) string {
	text := html.UnescapeString(htmlTagPattern.ReplaceAllString(description, " "))
	text = strings.TrimSpace(whitespacePattern.ReplaceAllString(text, " "))
	if text == "" {
		text = strings.TrimSpace(fileExtPattern.ReplaceAllString(strings.TrimPrefix(title, "File:"), ""))
	}
	if runes := []rune(text); len(runes) > 140 {
		text = strings.TrimSpace(string(runes[:140])) + "…"
	}
	return text
}

// getDestinationImages returns scenic images, with captions, from the destination's Wikipedia article.
func getDestinationImages(place string) ([]DestinationImage, error) {
	place = strings.TrimSpace(place)
	if place == "" || len(place) > maxDestinationPlace {
		return nil, fmt.Errorf("invalid destination")
	}
	refs, err := destinationImageRefs(place)
	if err != nil {
		return nil, err
	}
	var images []DestinationImage
	for _, ref := range refs {
		if dataURL, err := downloadDestinationImage(ref.ThumbURL); err == nil {
			images = append(images, DestinationImage{DataURL: dataURL, Caption: ref.Caption, PageURL: ref.PageURL})
		}
	}
	return images, nil
}

func destinationImageRefs(place string) ([]destinationImageRef, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(place)))
	listPath := filepath.Join(cacheDirectory, "photo-id", "destinations-v3", fmt.Sprintf("%x.json", hash))
	if cached, err := readFreshCacheFile(listPath, time.Now()); err == nil {
		var refs []destinationImageRef
		if json.Unmarshal(cached, &refs) == nil && len(refs) > 0 {
			return refs, nil
		}
	}

	// action=parse lists images in article order, so lead (usually scenic) images come first.
	listBody, err := fetchLimited(wikipediaAPIURL+"?"+url.Values{
		"action":    {"parse"},
		"format":    {"json"},
		"page":      {place},
		"redirects": {"1"},
		"prop":      {"images"},
	}.Encode(), 4<<20)
	if err != nil {
		return nil, fmt.Errorf("look up destination: %w", err)
	}
	var list wikiImageListResponse
	if err := json.Unmarshal(listBody, &list); err != nil {
		return nil, fmt.Errorf("parse destination response: %w", err)
	}
	var titles []string
	for _, name := range list.Parse.Images {
		if title := "File:" + strings.ReplaceAll(name, "_", " "); !excludedDestinationImage.MatchString(title) && len(titles) < 50 {
			titles = append(titles, title)
		}
	}
	if len(titles) == 0 {
		return nil, nil
	}

	query := url.Values{
		"action":              {"query"},
		"format":              {"json"},
		"titles":              {strings.Join(titles, "|")},
		"prop":                {"imageinfo"},
		"iiprop":              {"url|mime|size|extmetadata"},
		"iiextmetadatafilter": {"ImageDescription"},
		"iiurlwidth":          {"640"},
	}
	body, err := fetchLimited(wikipediaAPIURL+"?"+query.Encode(), 4<<20)
	if err != nil {
		return nil, fmt.Errorf("look up destination images: %w", err)
	}
	var response wikiImagesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse destination response: %w", err)
	}

	thumbs := map[string]destinationImageRef{}
	articleURL := "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(list.Parse.Title, " ", "_"))
	for _, page := range response.Query.Pages {
		if len(page.ImageInfo) == 0 {
			continue
		}
		info := page.ImageInfo[0]
		if info.Mime == "image/jpeg" && info.Width > info.Height {
			thumbs[page.Title] = destinationImageRef{
				ThumbURL: info.ThumbURL,
				Caption:  cleanCaption(imageDescription(info.ExtMetadata), page.Title),
				PageURL:  articleURL,
			}
		}
	}

	var refs []destinationImageRef
	for _, title := range scenicFirst(titles) {
		if len(refs) == maxDestinationImages {
			break
		}
		if ref, ok := thumbs[title]; ok {
			refs = append(refs, ref)
		}
	}
	if len(refs) > 0 {
		if encoded, err := json.Marshal(refs); err == nil {
			_ = writeCacheFile(listPath, encoded)
		}
	}
	return refs, nil
}

var scenicDestinationImage = regexp.MustCompile(`(?i)park|mountain|mount |lake|falls|river|canyon|glacier|beach|forest|bird|wildlife|landscape|valley|geyser|spring|reserve|island|coast|desert|volcano|sunset|bay|peak|tree|flower|cliff|lagoon|wetland|prairie|hill`)

// scenicFirst stably moves titles that look like nature or scenery ahead of the rest.
func scenicFirst(titles []string) []string {
	ordered := make([]string, 0, len(titles))
	for _, title := range titles {
		if scenicDestinationImage.MatchString(title) {
			ordered = append(ordered, title)
		}
	}
	for _, title := range titles {
		if !scenicDestinationImage.MatchString(title) {
			ordered = append(ordered, title)
		}
	}
	return ordered
}

func downloadDestinationImage(imageURL string) (string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != wikimediaImageHost && parsed.Host != wikimediaThumbHost) {
		return "", fmt.Errorf("unsupported image URL")
	}
	path, err := photoCachePath(imageURL)
	if err != nil {
		return "", err
	}
	data, err := readFreshCacheFile(path, time.Now())
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if data, err = fetchLimited(imageURL, photoFetchLimit); err != nil {
			return "", err
		}
		_ = writeCacheFile(path, data)
	}
	if mediaType := http.DetectContentType(data); mediaType != "image/jpeg" {
		return "", fmt.Errorf("image response is not a JPEG")
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data), nil
}

func fetchLimited(target string, limit int64) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", destinationUserAgent)
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d-byte limit", limit)
	}
	return data, nil
}
