package locallifecycle_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalMakeUpBuildsCanonicalRuntime(testingContext *testing.T) {
	repositoryRoot := locateRepositoryRoot(testingContext)
	runtimeDirectory := filepath.Join(testingContext.TempDir(), "runtime")
	command := exec.Command("make", "up")
	command.Dir = repositoryRoot
	command.Env = environmentWith(map[string]string{
		"LEDGER_LOCAL_RUNTIME_DIR": runtimeDirectory,
		"LEDGER_LOCAL_TEST_MODE":   "1",
		"UP_DRY_RUN":               "1",
	})
	output, err := command.CombinedOutput()
	if err != nil {
		testingContext.Fatalf("make up dry run: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"Ledger local runtime",
		"Browser URL: http://localhost:8000/",
		"gRPC address: localhost:50051",
		"Database: SQLite",
		"Local data: preserved by make down",
		"Dry run complete.",
	} {
		if !strings.Contains(string(output), expected) {
			testingContext.Fatalf("make up output does not contain %q: %s", expected, output)
		}
	}

	ledgerEnvironment := readEnvironment(testingContext, filepath.Join(runtimeDirectory, "ledger.env"))
	tauthEnvironment := readEnvironment(testingContext, filepath.Join(runtimeDirectory, "tauth.env"))
	requireEnvironment(testingContext, ledgerEnvironment, map[string]string{
		"DATABASE_URL":              "sqlite:///srv/data/ledger.db",
		"LEDGER_PUBLIC_ORIGIN":      "http://localhost:8000",
		"TAUTH_JWT_ISSUER":          "tauth",
		"TAUTH_LOGIN_PATH":          "/auth/google",
		"TAUTH_LOGOUT_PATH":         "/auth/logout",
		"TAUTH_NONCE_PATH":          "/auth/nonce",
		"TAUTH_SESSION_COOKIE_NAME": "ledger_local_session",
		"TAUTH_SESSION_PATH":        "/auth/session",
		"TAUTH_TENANT_ID":           "ledger-local",
		"TAUTH_URL":                 "http://localhost:8000",
	})
	requireEnvironment(testingContext, tauthEnvironment, map[string]string{
		"TAUTH_CONFIG_FILE":         "/config.yaml",
		"TAUTH_DATABASE_URL":        "sqlite:///data/tauth.db",
		"TAUTH_PUBLIC_ORIGIN":       "http://localhost:8000",
		"TAUTH_REFRESH_COOKIE_NAME": "ledger_local_refresh",
		"TAUTH_SESSION_COOKIE_NAME": "ledger_local_session",
		"TAUTH_TENANT_ID":           "ledger-local",
	})
	if ledgerEnvironment["TAUTH_GOOGLE_CLIENT_ID"] == "" || ledgerEnvironment["TAUTH_GOOGLE_CLIENT_ID"] != tauthEnvironment["TAUTH_GOOGLE_CLIENT_ID"] {
		testingContext.Fatal("local services do not share one Google client ID")
	}
	if ledgerEnvironment["TAUTH_JWT_SIGNING_KEY"] == "" || ledgerEnvironment["TAUTH_JWT_SIGNING_KEY"] != tauthEnvironment["TAUTH_JWT_SIGNING_KEY"] {
		testingContext.Fatal("local services do not share one signing key")
	}
	for _, name := range []string{"ledger.env", "tauth.env", "session-signing-key"} {
		information, statErr := os.Stat(filepath.Join(runtimeDirectory, name))
		if statErr != nil {
			testingContext.Fatalf("stat %s: %v", name, statErr)
		}
		if information.Mode().Perm() != 0o600 {
			testingContext.Fatalf("%s mode is %o", name, information.Mode().Perm())
		}
	}

	makefile := readFile(testingContext, filepath.Join(repositoryRoot, "Makefile"))
	if !strings.Contains(makefile, "up:\n\t@./demo/up.sh") || !strings.Contains(makefile, "down:\n\t@./demo/down.sh") {
		testingContext.Fatal("root Makefile does not own the local lifecycle commands")
	}
	compose := readFile(testingContext, filepath.Join(repositoryRoot, "demo", "docker-compose.yml"))
	for _, expected := range []string{
		"name: ledger-local",
		"${LEDGER_LOCAL_LEDGER_ENV_FILE:?LEDGER_LOCAL_LEDGER_ENV_FILE is required}",
		"${LEDGER_LOCAL_TAUTH_ENV_FILE:?LEDGER_LOCAL_TAUTH_ENV_FILE is required}",
		`"127.0.0.1:50051:50051"`,
		`"127.0.0.1:8000:8000"`,
	} {
		if !strings.Contains(compose, expected) {
			testingContext.Fatalf("local Compose file does not contain %q", expected)
		}
	}
	for _, obsolete := range []string{"profiles:", "computercat", "./configs/.env.ledger", "./configs/.env.tauth"} {
		if strings.Contains(compose, obsolete) {
			testingContext.Fatalf("local Compose file contains obsolete value %q", obsolete)
		}
	}
	tauthConfig := readFile(testingContext, filepath.Join(repositoryRoot, "demo", "configs", "tauth.config.yaml"))
	for _, expected := range []string{
		"enable_tenant_header_override: true",
		`id: "${TAUTH_TENANT_ID}"`,
		`jwt_signing_key: "${TAUTH_JWT_SIGNING_KEY}"`,
		`session_cookie_name: "${TAUTH_SESSION_COOKIE_NAME}"`,
	} {
		if !strings.Contains(tauthConfig, expected) {
			testingContext.Fatalf("local TAuth config does not contain %q", expected)
		}
	}
}

func TestLocalMakeDownTargetsOwnedProjectAndPreservesState(testingContext *testing.T) {
	repositoryRoot := locateRepositoryRoot(testingContext)
	temporaryDirectory := testingContext.TempDir()
	runtimeDirectory := filepath.Join(temporaryDirectory, "runtime")
	if err := os.MkdirAll(runtimeDirectory, 0o700); err != nil {
		testingContext.Fatalf("create runtime fixture: %v", err)
	}
	for _, name := range []string{"ledger.env", "tauth.env", "session-signing-key"} {
		if err := os.WriteFile(filepath.Join(runtimeDirectory, name), []byte("FIXTURE=value\n"), 0o600); err != nil {
			testingContext.Fatalf("write %s: %v", name, err)
		}
	}
	fakeBinaryDirectory := filepath.Join(temporaryDirectory, "bin")
	if err := os.MkdirAll(fakeBinaryDirectory, 0o700); err != nil {
		testingContext.Fatalf("create fake binary directory: %v", err)
	}
	dockerLogPath := filepath.Join(temporaryDirectory, "docker.log")
	fakeDockerPath := filepath.Join(fakeBinaryDirectory, "docker")
	fakeDocker := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$*\" >>\"${FAKE_DOCKER_LOG}\"\n"
	if err := os.WriteFile(fakeDockerPath, []byte(fakeDocker), 0o700); err != nil {
		testingContext.Fatalf("write fake docker: %v", err)
	}

	command := exec.Command("make", "down")
	command.Dir = repositoryRoot
	command.Env = environmentWith(map[string]string{
		"FAKE_DOCKER_LOG":          dockerLogPath,
		"LEDGER_LOCAL_RUNTIME_DIR": runtimeDirectory,
		"LEDGER_LOCAL_TEST_MODE":   "1",
		"PATH":                     fakeBinaryDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	output, err := command.CombinedOutput()
	if err != nil {
		testingContext.Fatalf("make down: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Ledger local runtime stopped. Local data remains available for the next start.") {
		testingContext.Fatalf("unexpected make down output: %s", output)
	}
	dockerLog := readFile(testingContext, dockerLogPath)
	expectedCommand := "--file " + filepath.Join(repositoryRoot, "demo", "docker-compose.yml") + " --project-name ledger-local down --remove-orphans"
	if !strings.Contains(dockerLog, expectedCommand) {
		testingContext.Fatalf("make down did not target the owned project: %s", dockerLog)
	}
	for _, name := range []string{"ledger.env", "tauth.env", "session-signing-key"} {
		if _, statErr := os.Stat(filepath.Join(runtimeDirectory, name)); statErr != nil {
			testingContext.Fatalf("make down did not preserve %s: %v", name, statErr)
		}
	}
}

func locateRepositoryRoot(testingContext *testing.T) string {
	testingContext.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		testingContext.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readEnvironment(testingContext *testing.T, path string) map[string]string {
	testingContext.Helper()
	file, err := os.Open(path)
	if err != nil {
		testingContext.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			testingContext.Fatalf("close %s: %v", path, closeErr)
		}
	}()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found || key == "" {
			testingContext.Fatalf("invalid environment line in %s", path)
		}
		if _, exists := values[key]; exists {
			testingContext.Fatalf("duplicate environment key %s", key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		testingContext.Fatalf("read %s: %v", path, err)
	}
	return values
}

func requireEnvironment(testingContext *testing.T, actual map[string]string, expected map[string]string) {
	testingContext.Helper()
	for key, value := range expected {
		if actual[key] != value {
			testingContext.Fatalf("unexpected %s value", key)
		}
	}
}

func readFile(testingContext *testing.T, path string) string {
	testingContext.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		testingContext.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func environmentWith(overrides map[string]string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, item)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}
