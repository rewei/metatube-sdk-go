package curlfetch

import (
	"os"
	"os/exec"
)

const opensslConfig = `openssl_conf = openssl_init
[openssl_init]
ssl_conf = ssl_sect
[ssl_sect]
system_default = system_default_sect
[system_default_sect]
MinProtocol = TLSv1.2
CipherString = DEFAULT:@SECLEVEL=0
`

// Fetch fetches a URL using curl with SECLEVEL=0 to bypass weak DH keys.
// Use for sites with outdated TLS configurations (e.g. kutikomiya.jp).
func Fetch(rawURL string, extraArgs ...string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "openssl-*.cnf")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.WriteString(opensslConfig); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, err
	}
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := []string{"-s", "-L", "--insecure", "-H", "User-Agent: Mozilla/5.0"}
	args = append(args, extraArgs...)
	args = append(args, rawURL)

	cmd := exec.Command("curl", args...)
	cmd.Env = append(os.Environ(), "OPENSSL_CONF="+tmpPath)
	return cmd.Output()
}
