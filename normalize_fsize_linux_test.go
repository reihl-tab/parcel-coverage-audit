package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const fsizeEnv = "COVERAGE_AUDIT_TEST_FSIZE_PATH"

// The partial write runs in a child process (this test binary re-executed),
// because RLIMIT_FSIZE is process-wide: in the test process it would also cut
// off the testing framework's own log and coverage writes. With fsizeEnv set,
// TestMain runs partialWriteChild instead of the tests.
func init() {
	if _, ok := os.LookupEnv(fsizeEnv); ok {
		testMainHook = partialWriteChild
	}
}

func partialWriteChild() {
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		fmt.Println("setup:", err)
		os.Exit(3)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 4, Max: old.Max}); err != nil {
		fmt.Println("setup:", err)
		os.Exit(3)
	}
	err := writeFileAtomic(os.Getenv(fsizeEnv), []byte("synthetic body longer than the limit\n"), 0600)
	if rerr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old); rerr != nil {
		fmt.Println("setup:", rerr)
		os.Exit(3)
	}
	fmt.Println("writeFileAtomic error:", err)
	os.Exit(0)
}

// TestWriteFileAtomicPartialWriteCleansUp makes the temp-file write stop part
// way (RLIMIT_FSIZE of 4 bytes, below the body length: the kernel accepts the
// first bytes, then refuses the rest with EFBIG) and checks that
// writeFileAtomic reports the error, publishes nothing at path and removes its
// temp file. Linux only (file name suffix).
func TestWriteFileAtomicPartialWriteCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.json")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), fsizeEnv+"="+path)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("child: %v, output %q", err, out.String())
	}
	if got := out.String(); !strings.HasPrefix(got, "writeFileAtomic error: ") || strings.Contains(got, "<nil>") {
		t.Fatalf("child output %q, want a write error", got)
	}
	if !strings.Contains(out.String(), "file too large") {
		t.Errorf("child output %q, want the EFBIG error from the partial write", out.String())
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("target exists after a partial write: %v", err)
	}
	assertNoStrayTemp(t, dir)
}
