package winadt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testSessionOptions builds SessionOptions with logging redirected into the
// test's temp dir (mirror of the deploy package's test helper).
func testSessionOptions(t *testing.T) SessionOptions {
	t.Helper()
	dir := t.TempDir()
	logDir := strings.ReplaceAll(filepath.Join(dir, "logs"), `\`, `\\`)
	overlay := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(overlay,
		[]byte("Toolkit:\n  LogPath: "+logDir+"\n  LogPathNoAdminRights: "+logDir+"\n"), 0o644))
	return SessionOptions{
		AppVendor:         "Contoso",
		AppName:           "Runner Test",
		AppVersion:        "1.0",
		ConfigOverlayPath: overlay,
	}
}

// runDeployment runs a Deployment capturing its exit code.
func runDeployment(t *testing.T, d *Deployment, args ...string) int {
	t.Helper()
	code := -1
	d.Args = append([]string{}, args...) // non-nil so os.Args (test flags) is never parsed
	d.Exit = func(c int) { code = c }
	d.Run(context.Background())
	return code
}
