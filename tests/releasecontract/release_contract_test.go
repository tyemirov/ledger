package releasecontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseLifecycleUsesRepositoryOwnedTooling(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	makefilePath := filepath.Join(repositoryRoot, "Makefile")
	makefile, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %s: %v", makefilePath, err)
	}
	makefileText := string(makefile)
	for _, contract := range []string{
		"RELEASE_HELPER := $(abspath $(CURDIR)/scripts/release/release_helper.py)",
		"RELEASE_TOOL_DIR := $(abspath $(CURDIR)/scripts/release)",
	} {
		if !strings.Contains(makefileText, contract) {
			t.Fatalf("Makefile does not declare %q", contract)
		}
	}
	if strings.Contains(makefileText, "agentSkills/gitrelease") {
		t.Fatal("Makefile still references sibling release tooling")
	}

	wrappers := map[string]string{
		"scripts/release.sh":         "${repo_root}/scripts/release/prepare_release.sh",
		"scripts/publish-release.sh": "${repo_root}/scripts/release/publish_release.sh",
	}
	for relativePath, localPipeline := range wrappers {
		path := filepath.Join(repositoryRoot, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		if !strings.Contains(text, localPipeline) {
			t.Fatalf("%s does not execute %s", relativePath, localPipeline)
		}
		if strings.Contains(text, "agentSkills/gitrelease") {
			t.Fatalf("%s still references sibling release tooling", relativePath)
		}
	}

	for _, script := range []string{
		"prepare_release.sh",
		"publish_release.sh",
		"release_helper.py",
		"prepare_container_artifact.sh",
		"publish_container_artifacts.sh",
	} {
		path := filepath.Join(repositoryRoot, "scripts", "release", script)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s is not executable", path)
		}
	}

	command := exec.Command("make", "--dry-run", "release", "container-artifacts", "publish")
	command.Dir = repositoryRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run release lifecycle: %v\n%s", err, output)
	}
	outputText := string(output)
	for _, entrypoint := range []string{
		"scripts/release.sh",
		"scripts/release/prepare_container_artifact.sh",
		"scripts/release/publish_container_artifacts.sh",
	} {
		if !strings.Contains(outputText, entrypoint) {
			t.Fatalf("dry-run output does not use %s:\n%s", entrypoint, outputText)
		}
	}
	if strings.Contains(outputText, "agentSkills/gitrelease") {
		t.Fatalf("dry-run output references sibling release tooling:\n%s", outputText)
	}
}
