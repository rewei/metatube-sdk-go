package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	kutikomiyaArchiveURL = "https://kutikomiya.jp/av-idol/archive/"
	minnanoSearchURL     = "https://www.minnano-av.com/search_result.php?search_scope=actress&search_word=%s&search=Go"
	minnanoActorURL      = "https://www.minnano-av.com/actress%s.html"
	outputFile           = "gfriends_slug.json"
)

var client = &http.Client{Timeout: 15 * time.Second}

func main() {
	fmt.Println("Fetching actress list from kutikomiya archive...")
	body, err := httpGet(kutikomiyaArchiveURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	re := regexp.MustCompile(`av-idol/([a-z][a-z-]+)/" title="([^（]+)`)
	matches := re.FindAllStringSubmatch(string(body), -1)

	results := make(map[string]string)
	seen := make(map[string]bool)
	// Collect slugs for alias lookup.
	type entry struct{ name, slug string }
	var entries []entry
	for _, match := range matches {
		if len(match) >= 3 {
			slug := match[1]
			name := match[2]
			if name != "" && !seen[name] {
				seen[name] = true
				results[name] = slug
				entries = append(entries, entry{name, slug})
			}
		}
	}

	existing := make(map[string]string)
	if data, err := os.ReadFile(outputFile); err == nil {
		json.Unmarshal(data, &existing)
	}

	fmt.Printf("Found %d actresses from kutikomiya\n", len(results))
	fmt.Println("Fetching aliases from MINNANO...")

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 5)
		added int
	)

	for _, e := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(name, slug string) {
			defer wg.Done()
			defer func() { <-sem }()

			aliases := fetchAliasesFromMinnano(name)
			mu.Lock()
			defer mu.Unlock()
			for _, alias := range aliases {
				if alias != "" && alias != name && !seen[alias] {
					seen[alias] = true
					results[alias] = slug
					added++
				}
			}
		}(e.name, e.slug)
	}
	wg.Wait()

	// Preserve manual entries.
	manualCount := 0
	for name, slug := range existing {
		if _, ok := results[name]; !ok {
			results[name] = slug
			manualCount++
		}
	}

	fmt.Printf("Done. %d total entries (%d aliases added, %d manual)\n", len(results), added, manualCount)

	data, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(outputFile, data, 0644)
	fmt.Printf("Written to %s\n", outputFile)
}

func fetchAliasesFromMinnano(name string) []string {
	searchURL := fmt.Sprintf(minnanoSearchURL, url.QueryEscape(name))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	re := regexp.MustCompile(`actress(\d+)\.html`)
	matches := re.FindStringSubmatch(finalURL)
	var actressID string
	if len(matches) >= 2 {
		actressID = matches[1]
	} else {
		body, _ := io.ReadAll(resp.Body)
		allMatches := re.FindAllStringSubmatch(string(body), -1)
		if len(allMatches) > 0 {
			actressID = allMatches[0][1]
		}
	}
	if actressID == "" {
		return nil
	}

	actorURL := fmt.Sprintf(minnanoActorURL, actressID)
	req, _ = http.NewRequest("GET", actorURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp2, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)

	// Check if the page name matches the keyword (or contains it as alias).
	re = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	nameMatch := re.FindSubmatch(body2)
	if len(nameMatch) >= 2 {
		actualName := string(nameMatch[1])
		if !strings.Contains(actualName, name) && !strings.Contains(string(body2), name) {
			return nil // wrong page
		}
	}

	// Extract aliases from the 別名 section in HTML.
	re = regexp.MustCompile(`別名</span><p>([^<]+)`)
	aliasMatches := re.FindAllSubmatch(body2, -1)
	if len(aliasMatches) < 2 {
		return nil // first entry is the main name, need at least 1 alias
	}
	// Skip first entry (main name), rest are aliases.
	var result []string
	for _, aliasMatch := range aliasMatches[1:] {
		aliasText := string(aliasMatch[1])
		// Extract name before the （reading）
		parts := strings.Split(aliasText, "（")[0]
		parts = strings.Split(parts, "(")[0]
		parts = strings.TrimSpace(parts)
		if parts != "" && parts != name {
			result = append(result, parts)
		}
	}
	return result
}

func httpGet(rawURL string) ([]byte, error) {
	cmd := exec.Command("curl", "-s", "-L",
		"--insecure",
		"--tlsv1.2",
		"--ciphers", "DHE-RSA-AES128-GCM-SHA256",
		"-H", "User-Agent: Mozilla/5.0",
		rawURL)
	tmpFile, err := os.CreateTemp("", "openssl-*.cnf")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.WriteString("openssl_conf = openssl_init\n[openssl_init]\nssl_conf = ssl_sect\n[ssl_sect]\nsystem_default = system_default_sect\n[system_default_sect]\nMinProtocol = TLSv1.2\nCipherString = DEFAULT:@SECLEVEL=0\n")
	tmpFile.Close()
	defer os.Remove(tmpPath)
	cmd.Env = append(os.Environ(), "OPENSSL_CONF="+tmpPath)
	return cmd.Output()
}