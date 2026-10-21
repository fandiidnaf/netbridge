// Package updater manages release verification, update notifications, and upgrade guidance.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner     = "fandiidnaf"
	repoName      = "netbridge"
	checkInterval = 24 * time.Hour
)

// ReleaseInfo holds the latest version details from GitHub.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

type cacheData struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version"`
	URL           string    `json:"url"`
}

// IsHomebrewInstall checks whether the running binary was installed via Homebrew.
func IsHomebrewInstall() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = realPath
	}
	exeLower := strings.ToLower(exe)
	return strings.Contains(exeLower, "/cellar/") ||
		strings.Contains(exeLower, "/opt/homebrew/") ||
		strings.Contains(exeLower, "/usr/local/cellar/") ||
		strings.Contains(exeLower, "homebrew-tap")
}

// FetchLatestRelease queries GitHub API for the latest published tag.
func FetchLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "netbridge-cli")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

// CheckUpdateNotice checks if a newer version exists (cached every 24h) and returns a friendly notice.
func CheckUpdateNotice(currentVersion string) string {
	cleanCurrent := strings.TrimPrefix(currentVersion, "v")
	cacheFile := getCacheFilePath()

	var cache cacheData
	if data, err := os.ReadFile(cacheFile); err == nil {
		_ = json.Unmarshal(data, &cache)
	}

	now := time.Now()
	if now.Sub(cache.LastCheck) > checkInterval {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		info, err := FetchLatestRelease(ctx)
		if err == nil && info.TagName != "" {
			cache.LastCheck = now
			cache.LatestVersion = strings.TrimPrefix(info.TagName, "v")
			cache.URL = info.HTMLURL
			saveCache(cacheFile, cache)
		}
	}

	if cache.LatestVersion != "" && IsNewer(cache.LatestVersion, cleanCurrent) {
		if IsHomebrewInstall() {
			return fmt.Sprintf("Notice: A newer version of NetBridge is available (v%s -> v%s). Run 'brew upgrade netbridge' to update.", cleanCurrent, cache.LatestVersion)
		}
		return fmt.Sprintf("Notice: A newer version of NetBridge is available (v%s -> v%s). Run 'netbridge update' to update.", cleanCurrent, cache.LatestVersion)
	}

	return ""
}

// RunUpdate performs an update check and outputs tailored update instructions or execution.
func RunUpdate(currentVersion string) error {
	cleanCurrent := strings.TrimPrefix(currentVersion, "v")
	fmt.Printf("Checking for updates (current version: v%s)...\n", cleanCurrent)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	info, err := FetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestClean := strings.TrimPrefix(info.TagName, "v")
	if !IsNewer(latestClean, cleanCurrent) {
		fmt.Printf("NetBridge is already up to date (v%s).\n", cleanCurrent)
		return nil
	}

	fmt.Printf("A new version is available: v%s (current: v%s)\n\n", latestClean, cleanCurrent)

	if IsHomebrewInstall() {
		fmt.Println("NetBridge is installed via Homebrew.")
		fmt.Println("To upgrade, execute:")
		fmt.Println("  brew update && brew upgrade netbridge")
		return nil
	}

	fmt.Println("To upgrade your installation:")
	fmt.Printf("  • Homebrew:  brew upgrade netbridge\n")
	fmt.Printf("  • Go:        go install github.com/%s/%s/cmd/netbridge@v%s\n", repoOwner, repoName, latestClean)
	fmt.Printf("  • Releases:  %s\n", info.HTMLURL)
	return nil
}

// IsNewer compares semantic versions (e.g., "1.0.2" vs "1.0.1").
func IsNewer(latest, current string) bool {
	lParts := strings.Split(latest, ".")
	cParts := strings.Split(current, ".")

	for i := 0; i < len(lParts) && i < len(cParts); i++ {
		var lNum, cNum int
		_, _ = fmt.Sscanf(lParts[i], "%d", &lNum)
		_, _ = fmt.Sscanf(cParts[i], "%d", &cNum)
		if lNum > cNum {
			return true
		}
		if lNum < cNum {
			return false
		}
	}
	return len(lParts) > len(cParts)
}

func getCacheFilePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		if runtime.GOOS == "darwin" {
			cacheDir = filepath.Join(os.Getenv("HOME"), "Library", "Caches")
		} else {
			cacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
		}
	}
	return filepath.Join(cacheDir, "netbridge", "update_check.json")
}

func saveCache(path string, cache cacheData) {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if data, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
