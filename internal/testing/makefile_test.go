package characterization

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runMakeDry(t *testing.T, target string) string {
	t.Helper()
	bin, err := lookupMake()
	if err != nil {
		t.Skipf("`make` not available on PATH: %v", err)
	}
	cmd := exec.Command(bin, "-f", makefilePath(t), "-n", target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s -f %s -n %s failed: %v\noutput:\n%s", bin, makefilePath(t), target, err, string(out))
	}
	return string(out)
}

func makefilePath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../Makefile")
	if err != nil {
		t.Fatalf("resolve Makefile path: %v", err)
	}
	return abs
}

func lookupMake() (string, error) {
	for _, candidate := range []string{"make", "mingw32-make", "gmake"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

func TestMakefileProtoTargetUsesProtoc(t *testing.T) {
	out := runMakeDry(t, "proto")

	if !strings.Contains(out, "protoc") {
		t.Errorf("make proto dry-run does not invoke protoc; got:\n%s", out)
	}
	if !strings.Contains(out, "--go_out=") {
		t.Errorf("make proto dry-run missing --go_out flag; got:\n%s", out)
	}
	if !strings.Contains(out, "proto/api.proto") {
		t.Errorf("make proto dry-run missing proto/api.proto input; got:\n%s", out)
	}
	if !strings.Contains(out, "github.com/jeriveromartinez/sofascore-scrapper") {
		t.Errorf("make proto dry-run missing go module path; got:\n%s", out)
	}
}

func TestMakefileProtoCheckVerifiesGitDiff(t *testing.T) {
	out := runMakeDry(t, "proto-check")

	if !strings.Contains(out, "protoc") {
		t.Errorf("make proto-check dry-run does not invoke protoc; got:\n%s", out)
	}
	if !strings.Contains(out, "git diff") {
		t.Errorf("make proto-check dry-run does not invoke git diff; got:\n%s", out)
	}
	if !strings.Contains(out, "internal/gen") {
		t.Errorf("make proto-check dry-run does not inspect internal/gen; got:\n%s", out)
	}
}

func TestMakefileProtoVerifyFlutterFetchesFlutterProto(t *testing.T) {
	out := runMakeDry(t, "proto-verify-flutter")

	if !strings.Contains(out, "curl") {
		t.Errorf("make proto-verify-flutter dry-run does not invoke curl; got:\n%s", out)
	}
	if !strings.Contains(out, "flutter-apptv") {
		t.Errorf("make proto-verify-flutter dry-run does not reference flutter-apptv; got:\n%s", out)
	}
	if !strings.Contains(out, "proto/api.proto") {
		t.Errorf("make proto-verify-flutter dry-run missing proto/api.proto path; got:\n%s", out)
	}
}
