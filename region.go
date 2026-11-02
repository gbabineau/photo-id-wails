package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var eBirdRegionInfoURL = "https://api.ebird.org/v2/ref/region/info/"

var regionCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{1,6}(-[A-Za-z0-9]{1,6}){0,2}$`)

// getRegionName returns the eBird name for a region code, or "" when it cannot be determined.
func getRegionName(ctx context.Context, regionCode string) string {
	regionCode = strings.ToUpper(strings.TrimSpace(regionCode))
	if !regionCodePattern.MatchString(regionCode) {
		return ""
	}

	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	cachePath := filepath.Join(cacheDirectory, "photo-id", "regions", regionCode+".txt")
	if cached, err := readFreshCacheFile(cachePath, time.Now()); err == nil {
		return string(cached)
	}

	apiKey := strings.TrimSpace(os.Getenv("EBIRDAPIKEY"))
	if apiKey == "" {
		return ""
	}
	endpoint := eBirdRegionInfoURL + url.PathEscape(regionCode) + "?regionNameFormat=detailednoqual"
	data, err := ebirdGet(ctx, http.DefaultClient, apiKey, endpoint)
	if err != nil {
		return ""
	}
	var info struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(data, &info); err != nil || info.Result == "" {
		return ""
	}
	_ = writeCacheFile(cachePath, []byte(info.Result))
	return info.Result
}

func (a *App) GetRegionName(regionCode string) string {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return getRegionName(ctx, regionCode)
}
