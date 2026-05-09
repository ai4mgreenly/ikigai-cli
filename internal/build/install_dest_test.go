package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestR_W23P_398Z_InstallPlacesBinaryInLocalBin verifies that
// `make install` puts the produced ikigai-cli binary at
// $HOME/.local/bin/ikigai-cli, creating the directory if absent
// and replacing any pre-existing file at that path.
//
// R-W23P-398Z: `make install` places the produced ikigai-cli binary
// at ~/.local/bin/ikigai-cli.
func TestR_W23P_398Z_InstallPlacesBinaryInLocalBin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping make install in -short mode")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skipf("make not available: %v", err)
	}

	root := repoRoot(t)
	fakeHome := t.TempDir()

	// Pre-populate the destination with a stale file to confirm it
	// gets replaced rather than left alone or refused.
	stalePath := filepath.Join(fakeHome, ".local", "bin", "ikigai-cli")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o755); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	// Run `make install` with HOME pointing at our temp dir so the
	// real ~/.local/bin is untouched.
	cmd := exec.Command("make", "install")
	cmd.Dir = root
	// Replace HOME but inherit everything else (PATH, GOCACHE, ...).
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		if len(kv) >= 5 && kv[:5] == "HOME=" {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+fakeHome)
	cmd.Env = out

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make install failed: %v\n%s", err, output)
	}

	info, err := os.Stat(stalePath)
	if err != nil {
		t.Fatalf("expected binary at %s: %v", stalePath, err)
	}
	if info.Size() < 1024 {
		t.Errorf("installed file at %s is %d bytes; stale 5-byte placeholder was not replaced with a real binary", stalePath, info.Size())
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed file at %s is not executable: mode=%v", stalePath, info.Mode())
	}
}
