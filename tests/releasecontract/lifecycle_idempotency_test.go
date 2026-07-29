package releasecontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const fixtureCommandTimeout = 30 * time.Second

func TestReleasePreparationIsIdempotentAtCurrentTag(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	prepareReleasePath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "prepare_release.sh"))
	if err != nil {
		t.Fatalf("resolve prepare release path: %v", err)
	}
	releaseHelperPath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "release_helper.py"))
	if err != nil {
		t.Fatalf("resolve release helper path: %v", err)
	}

	fixtureRoot := t.TempDir()
	fixtureRepository := filepath.Join(fixtureRoot, "ledger")
	fixtureRemote := filepath.Join(fixtureRoot, "origin.git")
	if err := os.MkdirAll(fixtureRepository, 0o755); err != nil {
		t.Fatalf("create fixture repository: %v", err)
	}
	runFixtureCommand(t, fixtureRoot, nil, "git", "init", "--bare", "--initial-branch=master", "--quiet", fixtureRemote)
	runFixtureCommand(t, fixtureRepository, nil, "git", "init", "--initial-branch=master", "--quiet")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.email", "fixture@example.invalid")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.name", "Fixture")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "add", "origin", fixtureRemote)
	writeFixtureFile(t, filepath.Join(fixtureRepository, "Makefile"), "ci:\n\t@true\n")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "CHANGELOG.md"), "# Changelog\n")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "source.txt"), "release source\n")
	runFixtureCommand(t, fixtureRepository, nil, "git", "add", ".")
	runFixtureCommand(t, fixtureRepository, nil, "git", "commit", "--quiet", "-m", "feat: release source")
	runFixtureCommand(t, fixtureRepository, nil, "git", "push", "--quiet", "--set-upstream", "origin", "master")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "set-head", "origin", "master")

	releaseEnvironment := map[string]string{
		"RELEASE_ARTIFACT_TARGETS": "",
		"RELEASE_HELPER":           releaseHelperPath,
	}
	runFixtureCommand(
		t,
		fixtureRepository,
		releaseEnvironment,
		prepareReleasePath,
		"--version",
		"v1.0.0",
	)
	releaseCommit := strings.TrimSpace(runFixtureCommand(t, fixtureRepository, nil, "git", "rev-parse", "HEAD"))
	artifactDirectory := strings.TrimSpace(
		runFixtureCommand(t, fixtureRepository, nil, "git", "rev-parse", "--git-path", "mprlab-release"),
	)
	if !filepath.IsAbs(artifactDirectory) {
		artifactDirectory = filepath.Join(fixtureRepository, artifactDirectory)
	}
	preparedFiles := readFixtureFiles(t, artifactDirectory)

	repeatedOutput := runFixtureCommand(t, fixtureRepository, releaseEnvironment, prepareReleasePath)

	if !strings.Contains(repeatedOutput, "Release v1.0.0 is already prepared") {
		t.Fatalf("repeated release did not report the verified no-op:\n%s", repeatedOutput)
	}
	if actualCommit := strings.TrimSpace(runFixtureCommand(t, fixtureRepository, nil, "git", "rev-parse", "HEAD")); actualCommit != releaseCommit {
		t.Fatalf("repeated release changed HEAD from %s to %s", releaseCommit, actualCommit)
	}
	if actualTags := strings.TrimSpace(runFixtureCommand(t, fixtureRepository, nil, "git", "tag", "--points-at", "HEAD")); actualTags != "v1.0.0" {
		t.Fatalf("unexpected release tags after retry: %q", actualTags)
	}
	if actualFiles := readFixtureFiles(t, artifactDirectory); !reflect.DeepEqual(actualFiles, preparedFiles) {
		t.Fatal("repeated release changed the prepared artifact")
	}
	if status := runFixtureCommand(t, fixtureRepository, nil, "git", "status", "--short"); status != "" {
		t.Fatalf("repeated release dirtied the fixture repository:\n%s", status)
	}
	if err := os.RemoveAll(artifactDirectory); err != nil {
		t.Fatalf("remove prepared artifact fixture: %v", err)
	}
	missingArtifactOutput := runFixtureCommand(t, fixtureRepository, releaseEnvironment, prepareReleasePath)
	if !strings.Contains(missingArtifactOutput, "release_artifact=missing") {
		t.Fatalf("release did not report the absent local artifact:\n%s", missingArtifactOutput)
	}
	if _, statErr := os.Stat(artifactDirectory); !os.IsNotExist(statErr) {
		t.Fatalf("release retry recreated or changed the absent artifact: %v", statErr)
	}
}

func TestGitHubPublicationDoesNotRewriteExactRelease(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	prepareReleasePath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "prepare_release.sh"))
	if err != nil {
		t.Fatalf("resolve prepare release path: %v", err)
	}
	releaseHelperPath, err := filepath.Abs(filepath.Join(repositoryRoot, "scripts", "release", "release_helper.py"))
	if err != nil {
		t.Fatalf("resolve release helper path: %v", err)
	}

	fixtureRoot := t.TempDir()
	fixtureRepository := filepath.Join(fixtureRoot, "ledger")
	fixtureRemote := filepath.Join(fixtureRoot, "origin.git")
	if err := os.MkdirAll(fixtureRepository, 0o755); err != nil {
		t.Fatalf("create fixture repository: %v", err)
	}
	runFixtureCommand(t, fixtureRoot, nil, "git", "init", "--bare", "--initial-branch=master", "--quiet", fixtureRemote)
	runFixtureCommand(t, fixtureRepository, nil, "git", "init", "--initial-branch=master", "--quiet")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.email", "fixture@example.invalid")
	runFixtureCommand(t, fixtureRepository, nil, "git", "config", "user.name", "Fixture")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "add", "origin", fixtureRemote)
	writeFixtureFile(t, filepath.Join(fixtureRepository, "Makefile"), "ci:\n\t@true\n")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "CHANGELOG.md"), "# Changelog\n")
	writeFixtureFile(t, filepath.Join(fixtureRepository, "source.txt"), "release source\n")
	runFixtureCommand(t, fixtureRepository, nil, "git", "add", ".")
	runFixtureCommand(t, fixtureRepository, nil, "git", "commit", "--quiet", "-m", "feat: release source")
	runFixtureCommand(t, fixtureRepository, nil, "git", "push", "--quiet", "--set-upstream", "origin", "master")
	runFixtureCommand(t, fixtureRepository, nil, "git", "remote", "set-head", "origin", "master")

	releaseEnvironment := map[string]string{
		"RELEASE_ARTIFACT_TARGETS": "",
		"RELEASE_HELPER":           releaseHelperPath,
	}
	runFixtureCommand(
		t,
		fixtureRepository,
		releaseEnvironment,
		prepareReleasePath,
		"--version",
		"v1.0.0",
	)

	fakeBinaryDirectory := filepath.Join(fixtureRoot, "bin")
	fakeReleaseState := filepath.Join(fixtureRoot, "release-state")
	fakeAssetDirectory := filepath.Join(fakeReleaseState, "assets")
	if err := os.MkdirAll(fakeBinaryDirectory, 0o755); err != nil {
		t.Fatalf("create fake binary directory: %v", err)
	}
	if err := os.MkdirAll(fakeAssetDirectory, 0o755); err != nil {
		t.Fatalf("create fake release state: %v", err)
	}
	fakeGitHubPath := filepath.Join(fakeBinaryDirectory, "gh")
	writeExecutableFixtureFile(t, fakeGitHubPath, `#!/usr/bin/env python3
import json
import os
import pathlib
import shutil
import sys

arguments = sys.argv[1:]
state = pathlib.Path(os.environ["FAKE_RELEASE_STATE"])
assets = state / "assets"
release_path = state / "release.json"
mutation_log = state / "mutations.log"

def option(name):
    index = arguments.index(name)
    return arguments[index + 1]

def emit(value):
    print(json.dumps(value))

if arguments[:2] == ["pr", "list"]:
    emit([])
    raise SystemExit(0)

if arguments[:2] == ["run", "list"]:
    emit([])
    raise SystemExit(0)

if arguments[:2] == ["release", "view"]:
    if not release_path.exists():
        print("release not found (HTTP 404)", file=sys.stderr)
        raise SystemExit(1)
    release = json.loads(release_path.read_text(encoding="utf-8"))
    if option("--json") == "assets":
        emit({
            "assets": [
                {"name": path.name, "size": path.stat().st_size}
                for path in sorted(assets.iterdir())
                if path.is_file()
            ]
        })
    else:
        emit(release)
    raise SystemExit(0)

if arguments[:2] == ["release", "create"]:
    version = arguments[2]
    release = {
        "tagName": version,
        "name": option("--title"),
        "body": pathlib.Path(option("--notes-file")).read_text(encoding="utf-8"),
        "publishedAt": "2026-07-28T23:59:00Z",
        "isDraft": False,
        "isPrerelease": False,
        "targetCommitish": "",
        "url": f"https://example.invalid/releases/{version}",
    }
    release_path.write_text(json.dumps(release), encoding="utf-8")
    with mutation_log.open("a", encoding="utf-8") as handle:
        handle.write("release create\n")
    raise SystemExit(0)

if arguments[:2] == ["release", "upload"]:
    for source in arguments[3:]:
        if source.startswith("--"):
            continue
        shutil.copy2(source, assets / pathlib.Path(source).name)
    with mutation_log.open("a", encoding="utf-8") as handle:
        handle.write("release upload\n")
    raise SystemExit(0)

if arguments[:2] == ["release", "download"]:
    pattern = option("--pattern")
    destination = pathlib.Path(option("--dir"))
    destination.mkdir(parents=True, exist_ok=True)
    shutil.copy2(assets / pattern, destination / pattern)
    raise SystemExit(0)

print(f"unexpected fake gh invocation: {arguments}", file=sys.stderr)
raise SystemExit(64)
`)

	publishEnvironment := map[string]string{
		"FAKE_RELEASE_STATE": fakeReleaseState,
		"PATH":               fakeBinaryDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	runFixtureCommand(
		t,
		fixtureRepository,
		publishEnvironment,
		"python3",
		releaseHelperPath,
		"publish-prepared-release",
	)
	mutationLogPath := filepath.Join(fakeReleaseState, "mutations.log")
	firstMutations, err := os.ReadFile(mutationLogPath)
	if err != nil {
		t.Fatalf("read first publication mutations: %v", err)
	}
	if actual := strings.Split(strings.TrimSpace(string(firstMutations)), "\n"); !reflect.DeepEqual(actual, []string{"release create", "release upload"}) {
		t.Fatalf("unexpected initial publication mutations: %v", actual)
	}

	runFixtureCommand(
		t,
		fixtureRepository,
		publishEnvironment,
		"python3",
		releaseHelperPath,
		"publish-prepared-release",
	)
	repeatedMutations, err := os.ReadFile(mutationLogPath)
	if err != nil {
		t.Fatalf("read repeated publication mutations: %v", err)
	}
	if !bytes.Equal(repeatedMutations, firstMutations) {
		t.Fatalf("repeated publication rewrote immutable GitHub state:\n%s", repeatedMutations)
	}
}

func TestContainerPublicationDoesNotRewriteExactRegistryState(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	publisherSourcePath, err := filepath.Abs(
		filepath.Join(repositoryRoot, "scripts", "release", "publish_container_artifacts.sh"),
	)
	if err != nil {
		t.Fatalf("resolve container publisher path: %v", err)
	}

	fixtureRoot := t.TempDir()
	fixtureRepository := filepath.Join(fixtureRoot, "ledger")
	fixtureReleaseScripts := filepath.Join(fixtureRepository, "scripts", "release")
	artifactDirectory := filepath.Join(fixtureRepository, ".git", "mprlab-release")
	containerDirectory := filepath.Join(artifactDirectory, "payloads", "containers", "ledger")
	fakeBinaryDirectory := filepath.Join(fixtureRoot, "bin")
	for _, directory := range []string{
		fixtureReleaseScripts,
		containerDirectory,
		fakeBinaryDirectory,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", directory, err)
		}
	}
	runFixtureCommand(t, fixtureRoot, nil, "git", "init", "--initial-branch=master", "--quiet", fixtureRepository)
	copyFixtureFile(
		t,
		publisherSourcePath,
		filepath.Join(fixtureReleaseScripts, "publish_container_artifacts.sh"),
		0o755,
	)
	writeExecutableFixtureFile(
		t,
		filepath.Join(fixtureReleaseScripts, "release_helper.py"),
		"#!/usr/bin/env python3\nimport sys\nraise SystemExit(0 if sys.argv[1:] == [\"verify-release-artifact\"] else 1)\n",
	)
	writeFixtureFile(t, filepath.Join(artifactDirectory, "manifest.json"), "{\"version\":\"v1.0.0\"}\n")
	amd64ArchiveContent := []byte("amd64 archive\n")
	arm64ArchiveContent := []byte("arm64 archive\n")
	amd64ArchivePath := filepath.Join(containerDirectory, "linux-amd64.tar")
	arm64ArchivePath := filepath.Join(containerDirectory, "linux-arm64.tar")
	if err := os.WriteFile(amd64ArchivePath, amd64ArchiveContent, 0o644); err != nil {
		t.Fatalf("write amd64 archive: %v", err)
	}
	if err := os.WriteFile(arm64ArchivePath, arm64ArchiveContent, 0o644); err != nil {
		t.Fatalf("write arm64 archive: %v", err)
	}
	amd64ArchiveHash := fmt.Sprintf("%x", sha256.Sum256(amd64ArchiveContent))
	arm64ArchiveHash := fmt.Sprintf("%x", sha256.Sum256(arm64ArchiveContent))
	writeFixtureFile(
		t,
		filepath.Join(containerDirectory, "container.json"),
		fmt.Sprintf(`{
  "artifact_kind": "mprlab.container",
  "image": "ghcr.io/example/ledger",
  "name": "ledger",
  "platforms": [
    {
      "archive": "payloads/containers/ledger/linux-amd64.tar",
      "image_id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "local_ref": "mprlab-release.local/ledger:v1.0.0-linux-amd64",
      "platform": "linux/amd64",
      "sha256": "%s",
      "token": "linux-amd64"
    },
    {
      "archive": "payloads/containers/ledger/linux-arm64.tar",
      "image_id": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "local_ref": "mprlab-release.local/ledger:v1.0.0-linux-arm64",
      "platform": "linux/arm64",
      "sha256": "%s",
      "token": "linux-arm64"
    }
  ],
  "schema_version": 1,
  "version": "v1.0.0"
}
`, amd64ArchiveHash, arm64ArchiveHash),
	)

	writeExecutableFixtureFile(t, filepath.Join(fakeBinaryDirectory, "gh"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "api" && "$2" == "user" ]]; then
  printf '%s\n' "fixture-user"
  exit 0
fi
if [[ "$1" == "auth" && "$2" == "token" ]]; then
  printf '%s\n' "fixture-token"
  exit 0
fi
echo "unexpected fake gh invocation: $*" >&2
exit 64
`)
	writeExecutableFixtureFile(t, filepath.Join(fakeBinaryDirectory, "docker"), `#!/usr/bin/env bash
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
if [[ "$1" == "login" || "$1" == "load" || "$1" == "tag" ]]; then
  exit 0
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  if [[ "$3" == *"linux-amd64" ]]; then
    printf '%s\n' "${amd64_digest}"
  else
    printf '%s\n' "${arm64_digest}"
  fi
  exit 0
fi
if [[ "$1" == "push" ]]; then
  printf 'push %s\n' "$2" >>"${FAKE_DOCKER_MUTATION_LOG}"
  exit 0
fi
if [[ "$1" == "buildx" && "$2" == "imagetools" && "$3" == "create" && "$4" == "--tag" ]]; then
  printf 'create %s\n' "$5" >>"${FAKE_DOCKER_MUTATION_LOG}"
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

echo "unexpected fake docker invocation: $*" >&2
exit 64
`)

	mutationLogPath := filepath.Join(fixtureRoot, "docker-mutations.log")
	publishEnvironment := map[string]string{
		"FAKE_DOCKER_MUTATION_LOG": mutationLogPath,
		"PATH":                     fakeBinaryDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	publisherPath := filepath.Join(fixtureReleaseScripts, "publish_container_artifacts.sh")
	publishOutput := runFixtureCommand(t, fixtureRepository, publishEnvironment, publisherPath)
	for _, expectedOutput := range []string{"no push needed", "no manifest update needed", "no update needed"} {
		if !strings.Contains(publishOutput, expectedOutput) {
			t.Fatalf("container retry did not report %q:\n%s", expectedOutput, publishOutput)
		}
	}
	if mutations, readErr := os.ReadFile(mutationLogPath); readErr == nil {
		t.Fatalf("container retry mutated exact registry state:\n%s", mutations)
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("read container mutation log: %v", readErr)
	}
}

func runFixtureCommand(
	t *testing.T,
	directory string,
	environment map[string]string,
	name string,
	arguments ...string,
) string {
	t.Helper()
	commandContext, cancel := context.WithTimeout(context.Background(), fixtureCommandTimeout)
	defer cancel()
	command := exec.CommandContext(commandContext, name, arguments...)
	command.Dir = directory
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if commandContext.Err() != nil {
		t.Fatalf("%s timed out:\n%s", name, output.String())
	}
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(arguments, " "), err, output.String())
	}
	return output.String()
}

func writeFixtureFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeExecutableFixtureFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func copyFixtureFile(t *testing.T, sourcePath string, destinationPath string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	if err := os.WriteFile(destinationPath, content, mode); err != nil {
		t.Fatalf("write %s: %v", destinationPath, err)
	}
}

func readFixtureFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[relativePath] = content
		return nil
	})
	if err != nil {
		t.Fatalf("read fixture files under %s: %v", root, err)
	}
	return files
}
