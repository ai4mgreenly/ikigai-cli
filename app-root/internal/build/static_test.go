package build_test

import (
	"debug/elf"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestR_10VP_QZBQ_StaticallyLinkedBinary builds the project via the
// project Makefile and verifies the resulting binary has no dynamic
// library dependencies — i.e. it is statically linked.
//
// R-10VP-QZBQ: the build artifact is a single statically linked binary
// per supported OS/arch, with no runtime dependencies.
func TestR_10VP_QZBQ_StaticallyLinkedBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("ELF check only meaningful on linux; have %s", runtime.GOOS)
	}

	root := repoRoot(t)

	cmd := exec.Command("make", "clean", "build")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make build failed: %v\n%s", err, out)
	}

	bin := filepath.Join(root, "bin", "ikigai-cli")
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatalf("open elf %s: %v", bin, err)
	}
	defer f.Close()

	libs, err := f.ImportedLibraries()
	if err != nil {
		t.Fatalf("imported libraries: %v", err)
	}
	if len(libs) != 0 {
		t.Errorf("binary has dynamic library dependencies, expected static: %v", libs)
	}

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			t.Errorf("binary has PT_INTERP segment; statically linked binaries should not request a dynamic loader")
			break
		}
	}
}
