package locallifecycle_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedMigrationCommand(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ledgerd")
	build := exec.Command("go", "build", "-o", executable, "./cmd/credit")
	build.Dir = locateRepositoryRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	command := exec.Command(executable, "migrate-user-accounts")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unknown command") {
		t.Fatalf("removed command must fail: %v: %s", err, output)
	}
}
