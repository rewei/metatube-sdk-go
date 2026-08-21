package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

const (
	filetreeURL = "https://raw.githubusercontent.com/rewei/avatars/master/Filetree.json"
	searchURL   = "https://kutikomiya.jp/search/av-idol/%s/"
	outputFile  = "gfriends_slug.json"
)

func main() {
	fmt.Println("Fetching Filetree.json...")
	names, err := fetchActorNames()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching Filetree.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d actors\n", len(names))

	// Load existing mapping file to preserve manual entries.
	existing := make(map[string]string)
	if data, err := os.ReadFile(outputFile); err == nil {
		json.Unmarshal(data, &existing)
		fmt.Printf("Loaded %d existing entries from %s\n", len(existing), outputFile)
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 5)
		results = make(map[string]string)
		errors  int
	)

	for _, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(n string) {
			defer wg.Done()
			defer func() { <-sem }()

			slug, err := resolveSlug(n)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errors++
				fmt.Printf("[ERROR] %s: %v\n", n, err)
				// Keep existing entry on error.
				if s, ok := existing[n]; ok {
					results[n] = s
				}
				return
			}
			if slug != "" {
				results[n] = slug
				fmt.Printf("[OK] %s -> %s\n", n, slug)
			} else {
				fmt.Printf("[SKIP] %s: no slug found\n", n)
				// Keep existing entry on skip.
				if s, ok := existing[n]; ok {
					results[n] = s
				}
			}
		}(name)
	}
	wg.Wait()

	// Preserve manual entries not in Filetree.json.
	for name, slug := range existing {
		if _, ok := results[name]; !ok {
			results[name] = slug
			fmt.Printf("[KEPT] %s -> %s (manual)\n", name, slug)
		}
	}

	fmt.Printf("\nDone. %d slugs found, %d errors\n", len(results), errors)

	if len(results) == 0 {
		fmt.Println("No slugs found, nothing to write.")
		return
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written to %s\n", outputFile)
}

func fetchActorNames() ([]string, error) {
	body, err := httpGet(filetreeURL)
	if err != nil {
		return nil, err
	}

	var ft struct {
		Content map[string]map[string]string `json:"Content"`
	}
	if err := json.Unmarshal(body, &ft); err != nil {
		return nil, err
	}

	var names []string
	for _, category := range ft.Content {
		for name := range category {
			name = strings.TrimSuffix(name, ".jpg")
			names = append(names, name)
		}
	}
	return names, nil
}

func resolveSlug(name string) (string, error) {
	searchURL := fmt.Sprintf(searchURL, url.QueryEscape(name))

	body, err := httpGet(searchURL)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`/av-idol/[a-z][a-z-]+/`)
	matches := re.FindAll(body, -1)
	skip := map[string]bool{
		"photo-album": true,
		"archive":     true,
		"ranking":     true,
		"bust":        true,
		"yomi":        true,
	}
	for _, match := range matches {
		slug := strings.TrimPrefix(string(match), "/av-idol/")
		slug = strings.TrimSuffix(slug, "/")
		if slug == "" || skip[slug] {
			continue
		}
		return slug, nil
	}

	return "", nil
}

func httpGet(rawURL string) ([]byte, error) {
	cmd := exec.Command("curl", "-s",
		"--insecure",
		"--tlsv1.2",
		"--ciphers", "DHE-RSA-AES128-GCM-SHA256",
		rawURL)
	cmd.Env = append(os.Environ(), "OPENSSL_CONF=/dev/null")
	// Create a temp OpenSSL config with SECLEVEL=0 to allow weak DH keys.
	tmpFile, err := os.CreateTemp("", "openssl-*.cnf")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.WriteString("openssl_conf = openssl_init\n[openssl_init]\nssl_conf = ssl_sect\n[ssl_sect]\nsystem_default = system_default_sect\n[system_default_sect]\nMinProtocol = TLSv1.2\nCipherString = DEFAULT:@SECLEVEL=0\n")
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd.Env = append(os.Environ(), "OPENSSL_CONF="+tmpPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("curl failed: %v", err)
	}
	return output, nil
}