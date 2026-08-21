package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
)

const (
	kutikomiyaArchiveURL = "https://kutikomiya.jp/av-idol/archive/"
	outputFile           = "gfriends_slug.json"
)

func main() {
	fmt.Println("Fetching actress list from kutikomiya archive...")
	body, err := httpGet(kutikomiyaArchiveURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Extract slug and name pairs: av-idol/<slug>/" title="<name>（<reading>）"
	re := regexp.MustCompile(`av-idol/([a-z][a-z-]+)/" title="([^（]+)`)
	matches := re.FindAllStringSubmatch(string(body), -1)

	results := make(map[string]string)
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) >= 3 {
			slug := match[1]
			name := match[2]
			if name != "" && !seen[name] {
				seen[name] = true
				results[name] = slug
			}
		}
	}

	// Load existing mapping file to preserve manual entries.
	existing := make(map[string]string)
	if data, err := os.ReadFile(outputFile); err == nil {
		json.Unmarshal(data, &existing)
	}

	// Merge: new results take priority, but manual entries are preserved.
	manualCount := 0
	for name, slug := range existing {
		if _, ok := results[name]; !ok {
			results[name] = slug
			manualCount++
		}
	}

	fmt.Printf("Found %d actresses from kutikomiya\n", len(results))
	if manualCount > 0 {
		fmt.Printf("Preserved %d manual entries\n", manualCount)
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(outputFile, data, 0644)
	fmt.Printf("Written to %s\n", outputFile)
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