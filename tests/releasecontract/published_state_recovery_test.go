package releasecontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedReleaseIsVerifiableWithoutLocalArtifact(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	publishReleasePath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "publish_release.sh"))
	if err != nil {
		t.Fatalf("resolve release publisher path: %v", err)
	}
	releaseHelperPath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "release_helper.py"))
	if err != nil {
		t.Fatalf("resolve release helper path: %v", err)
	}
	containerPublisherPath, err := filepath.Abs(
		filepath.Join(repositoryRoot, "scripts", "release", "publish_container_artifacts.sh"),
	)
	if err != nil {
		t.Fatalf("resolve container publisher path: %v", err)
	}

	fixtureRoot := t.TempDir()
	fixtureRepository := filepath.Join(fixtureRoot, "ledger")
	fixtureRemote := filepath.Join(fixtureRoot, "origin.git")
	fakeBinaryDirectory := filepath.Join(fixtureRoot, "bin")
	fakeReleaseState := filepath.Join(fixtureRoot, "release-state")
	fakeAssetDirectory := filepath.Join(fakeReleaseState, "assets")
	for _, directory := range []string{fixtureRepository, fakeBinaryDirectory, fakeAssetDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", directory, err)
		}
	}
	runFixtureCommand(t, fixtureRoot, nil, "git", "init", "--bare", "--initial-branch=master", "--quiet", fixtureRemote)
	runFixtureCommand(t, fixtureRepository, nil, "git", "init", "--initial-branch=master", "--quiet")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.email", "fixture@example.invalid")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.name", "Fixture")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "add", "origin", fixtureRemote)
	writeFixtureFile(t, filepath.Join(fixtureRepository, "CHANGELOG.md"), "# Changelog\n")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "source.txt"), "release source\n")
	runFixtureCommand(t, fixtureRepository, nil, "git", "add", ".")
	runFixtureCommand(t, fixtureRepository, nil, "git", "commit", "--quiet", "-m", "feat: release source")
	sourceCommit := strings.TrimSpace(runFixtureCommand(t, fixtureRepository, nil, "git", "rev-parse", "HEAD"))

	releaseNotes := "## [v1.0.0] - 2026-07-28\n\n- feat: release source\n"
	writeFixtureFile(t, filepath.Join(fixtureRepository, "CHANGELOG.md"), "# Changelog\n\n"+releaseNotes)
	runFixtureCommand(t, fixtureRepository, nil, "git", "add", "CHANGELOG.md")
	runFixtureCommand(t, fixtureRepository, nil, "git", "commit", "--quiet", "-m", "Release v1.0.0")
	releaseCommit := strings.TrimSpace(runFixtureCommand(t, fixtureRepository, nil, "git", "rev-parse", "HEAD"))
	runFixtureCommand(t, fixtureRepository, nil, "git", "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runFixtureCommand(t, fixtureRepository, nil, "git", "push", "--quiet", "--set-upstream", "origin", "master")
	runFixtureCommand(t, fixtureRepository, nil, "git", "push", "--quiet", "origin", "refs/tags/v1.0.0")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "set-head", "origin", "master")

	notesHash := fmt.Sprintf("%x", sha256.Sum256([]byte(releaseNotes)))
	releaseManifest := fmt.Sprintf(`{
  "artifact_kind": "mprlab.release",
  "default_branch": "master",
  "notes_sha256": "%s",
  "payloads": [],
  "release_commit": "%s",
  "release_timestamp": "2026-07-28T14:36:13-07:00",
  "schema_version": 2,
  "source_commit": "%s",
  "version": "v1.0.0"
}
`, notesHash, releaseCommit, sourceCommit)
	writeFixtureFile(t, filepath.Join(fakeAssetDirectory, "manifest.json"), releaseManifest)
	writeFixtureFile(
		t,
		filepath.Join(fakeReleaseState, "release.json"),
		fmt.Sprintf(`{
  "body": %q,
  "isDraft": false,
  "isPrerelease": false,
  "name": "Release v1.0.0",
  "publishedAt": "2026-07-28T23:59:00Z",
  "tagName": "v1.0.0",
  "targetCommitish": "master",
  "url": "https://example.invalid/releases/v1.0.0"
}
`, releaseNotes),
	)
	writeExecutableFixtureFile(t, filepath.Join(fakeBinaryDirectory, "gh"), `#!/usr/bin/env python3
import json
import os
import pathlib
import shutil
import sys

arguments = sys.argv[1:]
state = pathlib.Path(os.environ["FAKE_RELEASE_STATE"])
assets = state / "assets"
release_path = state / "release.json"

def option(name):
    index = arguments.index(name)
    return arguments[index + 1]

def emit(value):
    print(json.dumps(value))

if arguments[:2] == ["pr", "list"] or arguments[:2] == ["run", "list"]:
    emit([])
    raise SystemExit(0)

if arguments[:2] == ["release", "view"]:
    if option("--json") == "assets":
        emit({
            "assets": [
                {"name": path.name, "size": path.stat().st_size}
                for path in sorted(assets.iterdir())
                if path.is_file()
            ]
        })
    else:
        emit(json.loads(release_path.read_text(encoding="utf-8")))
    raise SystemExit(0)

if arguments[:2] == ["release", "download"]:
    pattern = option("--pattern")
    destination = pathlib.Path(option("--dir"))
    destination.mkdir(parents=True, exist_ok=True)
    shutil.copy2(assets / pattern, destination / pattern)
    raise SystemExit(0)

with (state / "mutations.log").open("a", encoding="utf-8") as handle:
    handle.write(" ".join(arguments) + "\n")
raise SystemExit(64)
`)
	writeExecutableFixtureFile(t, filepath.Join(fakeBinaryDirectory, "docker"), exactPublishedRegistryFixture())

	publishEnvironment := map[string]string{
		"DOCKER_IMAGE":             "ghcr.io/example/ledger",
		"FAKE_DOCKER_MUTATION_LOG": filepath.Join(fixtureRoot, "docker-mutations.log"),
		"FAKE_RELEASE_STATE":       fakeReleaseState,
		"PATH":                     fakeBinaryDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PUBLISH_PLATFORMS":        "linux/amd64,linux/arm64",
		"RELEASE_HELPER":           releaseHelperPath,
	}
	releaseOutput := runFixtureCommand(t, fixtureRepository, publishEnvironment, publishReleasePath)
	if !strings.Contains(releaseOutput, "published_release_already_verified") {
		t.Fatalf("release publisher did not verify completed remote state:\n%s", releaseOutput)
	}
	containerOutput := runFixtureCommand(t, fixtureRepository, publishEnvironment, containerPublisherPath)
	if !strings.Contains(containerOutput, "already published with exact platform manifests") {
		t.Fatalf("container publisher did not verify completed remote state:\n%s", containerOutput)
	}
	for _, mutationLogPath := range []string{
		filepath.Join(fakeReleaseState, "mutations.log"),
		filepath.Join(fixtureRoot, "docker-mutations.log"),
	} {
		if mutations, readErr := os.ReadFile(mutationLogPath); readErr == nil {
			t.Fatalf("published-state recovery mutated external state:\n%s", mutations)
		} else if !os.IsNotExist(readErr) {
			t.Fatalf("read mutation log %s: %v", mutationLogPath, readErr)
		}
	}
}

func TestDeployReportsMissingPublishedImage(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	deployPath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "deploy.sh"))
	if err != nil {
		t.Fatalf("resolve deploy path: %v", err)
	}

	fixtureRoot := t.TempDir()
	fixtureRepository := filepath.Join(fixtureRoot, "ledger")
	fakeGatewayDirectory := filepath.Join(fixtureRoot, "gateway")
	fakeBinaryDirectory := filepath.Join(fixtureRoot, "bin")
	for _, directory := range []string{fixtureRepository, fakeGatewayDirectory, fakeBinaryDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", directory, err)
		}
	}
	runFixtureCommand(t, fixtureRepository, nil, "git", "init", "--initial-branch=master", "--quiet")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.email", "fixture@example.invalid")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.name", "Fixture")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "CHANGELOG.md"), "# Changelog\n")
	runFixtureCommand(t, fixtureRepository, nil, "git", "add", ".")
	runFixtureCommand(t, fixtureRepository, nil, "git", "commit", "--quiet", "-m", "Release v1.0.0")
	runFixtureCommand(t, fixtureRepository, nil, "git", "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	writeExecutableFixtureFile(t, filepath.Join(fakeBinaryDirectory, "docker"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "buildx" && "$2" == "version" ]]; then
  exit 0
fi
if [[ "$1" == "buildx" && "$2" == "imagetools" && "$3" == "inspect" ]]; then
  echo "$4: manifest unknown" >&2
  exit 1
fi
exit 64
`)

	commandContext, cancel := context.WithTimeout(context.Background(), fixtureCommandTimeout)
	defer cancel()
	command := exec.CommandContext(
		commandContext,
		deployPath,
		"--gateway-dir",
		fakeGatewayDirectory,
		"--skip-backend",
	)
	command.Dir = fixtureRepository
	command.Env = append(
		os.Environ(),
		"PATH="+fakeBinaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	runErr := command.Run()
	if commandContext.Err() != nil {
		t.Fatalf("deploy image verification timed out:\n%s", output.String())
	}
	if runErr == nil {
		t.Fatalf("deploy accepted a missing release image:\n%s", output.String())
	}
	expectedError := "error: ghcr.io/tyemirov/ledger:v1.0.0 is not published; run make publish"
	if !strings.Contains(output.String(), expectedError) {
		t.Fatalf("deploy did not report the missing immutable image:\n%s", output.String())
	}
}

func exactPublishedRegistryFixture() string {
	return `#!/usr/bin/env bash
set -euo pipefail

amd64_ref="ghcr.io/example/ledger:v1.0.0-linux-amd64"
arm64_ref="ghcr.io/example/ledger:v1.0.0-linux-arm64"
version_ref="ghcr.io/example/ledger:v1.0.0"
latest_ref="ghcr.io/example/ledger:latest"
amd64_digest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
arm64_digest="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
version_digest="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

emit_digest() {
  printf 'Name:      %s\n' "$1"
  printf 'Digest:    %s\n' "$2"
}

emit_amd64_raw() {
  printf '%s\n' '{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","platform":{"os":"linux","architecture":"amd64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","platform":{"os":"unknown","architecture":"unknown"}}]}'
}

emit_arm64_raw() {
  printf '%s\n' '{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","platform":{"os":"unknown","architecture":"unknown"}}]}'
}

emit_version_raw() {
  printf '%s\n' '{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","platform":{"os":"linux","architecture":"amd64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","platform":{"os":"unknown","architecture":"unknown"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","platform":{"os":"unknown","architecture":"unknown"}}]}'
}

if [[ "$1" == "buildx" && "$2" == "version" ]]; then
  printf '%s\n' "github.com/docker/buildx fixture"
  exit 0
fi
if [[ "$1" == "buildx" && "$2" == "imagetools" && "$3" == "inspect" ]]; then
  image_ref="$4"
  raw="false"
  [[ "${5:-}" == "--raw" ]] && raw="true"
  case "${image_ref}" in
    "${amd64_ref}")
      if [[ "${raw}" == "true" ]]; then emit_amd64_raw; else emit_digest "${image_ref}" "${amd64_digest}"; fi
      ;;
    "${arm64_ref}")
      if [[ "${raw}" == "true" ]]; then emit_arm64_raw; else emit_digest "${image_ref}" "${arm64_digest}"; fi
      ;;
    "${version_ref}"|"${latest_ref}")
      if [[ "${raw}" == "true" ]]; then emit_version_raw; else emit_digest "${image_ref}" "${version_digest}"; fi
      ;;
    *)
      echo "unexpected fake registry ref: ${image_ref}" >&2
      exit 64
      ;;
  esac
  exit 0
fi

printf '%s\n' "$*" >>"${FAKE_DOCKER_MUTATION_LOG}"
exit 64
`
}
