package lifecyclecontract_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestVersionlessLifecycleContract(testingContext *testing.T) {
	testingContext.Parallel()
	repositoryRoot := locateRepositoryRoot(testingContext)
	manifestBytes := readFile(testingContext, filepath.Join(repositoryRoot, ".mprlab", "deploy", "resources.yml"))
	manifest := currentManifestContract(testingContext, manifestBytes)
	requireCurrentContractYAML(testingContext, "release", manifest["release"], `scheme: semver`)
	environment := currentContractMap(testingContext, manifest["environment"])
	requireCurrentContractYAML(testingContext, "environment.source", environment["source"], `.mprlab/deploy/.env`)
	requireCurrentContractYAML(testingContext, "environment.private_inputs", environment["private_inputs"], `
- {id: database-url, variable: DATABASE_URL}
- {id: deploy-sudo-password-tutosh, variable: DEPLOY_SUDO_PASSWORD_TUTOSH}
- {id: mprlab-tauth-management-url, variable: MPRLAB_TAUTH_MANAGEMENT_URL}
- {id: mprlab-tauth-provisioning-credentials, variable: MPRLAB_TAUTH_PROVISIONING_CREDENTIALS}
- {id: tauth-google-client-id, variable: TAUTH_GOOGLE_CLIENT_ID}
- {id: tauth-jwt-signing-key, variable: TAUTH_JWT_SIGNING_KEY}
`)
	requireCurrentContractYAML(testingContext, "owner", manifest["owner"], `ledger`)
	lifecycle := currentContractMap(testingContext, manifest["lifecycle"])
	requireCurrentContractYAML(testingContext, "lifecycle.defaults", lifecycle["defaults"], `timeouts: {startup: 30s, completion: 30m, readiness: 30s, request: 15s, shutdown: 5s}`)
	requireCurrentContractYAML(testingContext, "lifecycle.operations", lifecycle["operations"], `
- {name: app_lifecycle.repository_read, polling: {initial: 1s, maximum: 5s, increment: 0s, multiplier: 2, attempt_limit: 360}}
- {name: app-release, polling: {initial: 1s, maximum: 5s, increment: 0s, multiplier: 2, attempt_limit: 360}}
- {name: app-publish, polling: {initial: 1s, maximum: 5s, increment: 0s, multiplier: 2, attempt_limit: 360}}
- {name: app-deploy, polling: {initial: 1s, maximum: 5s, increment: 0s, multiplier: 2, attempt_limit: 360}}
`)
	requireCurrentContractYAML(testingContext, "lifecycle.observations", lifecycle["observations"], `http: [{id: api-health, protocol: http, url: https://ledger-api.mprlab.com/healthz, expected_status: 200}]`)
	artifacts := currentContractMap(testingContext, manifest["artifacts"])
	requireCurrentContractYAML(testingContext, "artifacts.container_images", artifacts["container_images"], `[{id: ledger-image, build: {context: ., dockerfile: Dockerfile, platforms: [linux/amd64, linux/arm64]}}]`)
	requireCurrentContractYAML(testingContext, "artifacts.web_bundles", artifacts["web_bundles"], `[{id: frontend, format: directory, source: {kind: container, context: ., dockerfile: Dockerfile, target: pages}}]`)
	publications := currentContractMap(testingContext, manifest["publications"])
	requireCurrentContractYAML(testingContext, "publications.ghcr", publications["ghcr"], `images: [{id: ledger-image, artifact: ledger-image, repository: ghcr.io/tyemirov/ledger}]`)
	requireCurrentContractYAML(testingContext, "publications.github_pages", publications["github_pages"], `sites: [{id: frontend, artifact: frontend, repository: tyemirov/ledger, branch: gh-pages, domain: ledger.mprlab.com, marker: {path: /.mprlab-release.json}}]`)
	targets := currentContractMap(testingContext, manifest["targets"])
	requireCurrentContractYAML(testingContext, "targets.access", targets["access"], `hosts: [{host: tutosh, elevation_password: {input: deploy-sudo-password-tutosh}}]`)
	compose := currentContractMap(testingContext, targets["docker_compose"])
	if projects, ok := compose["projects"].([]any); !ok || len(projects) != 1 {
		testingContext.Fatal("exactly one runtime project is required")
	}
	project := currentContractDeclaration(testingContext, compose["projects"], "runtime")
	if _, exists := project["placement"]; exists {
		testingContext.Fatal("Compose placement must exist only on services")
	}
	if _, exists := project["profiles"]; exists {
		testingContext.Fatal("Compose project profiles must be absent")
	}
	requireCurrentContractYAML(testingContext, "runtime.retired_services", project["retired_services"], `[{project: mprlab-nginx-gateway, service: ledger-api}]`)
	requireCurrentContractYAML(testingContext, "runtime.volumes", project["volumes"], `[{id: data, name: ledger-data, retention: retain}]`)
	if services, ok := project["services"].([]any); !ok || len(services) != 1 {
		testingContext.Fatal("exactly one ledger-api service is required")
	}
	service := currentContractDeclaration(testingContext, project["services"], "ledger-api")
	requireCurrentContractYAML(testingContext, "ledger-api.image", service["image"], `publication: ledger-image`)
	requireCurrentContractYAML(testingContext, "ledger-api.placement", service["placement"], `group: gateway
cardinality: one`)
	requireCurrentContractYAML(testingContext, "ledger-api.environment", service["environment"], `
- {name: DATABASE_URL, input: database-url}
- {name: LEDGER_PUBLIC_ORIGIN, value: https://ledger.mprlab.com}
- {name: TAUTH_JWT_SIGNING_KEY, input: tauth-jwt-signing-key}
- {name: TAUTH_GOOGLE_CLIENT_ID, input: tauth-google-client-id}
- {name: TAUTH_SESSION_COOKIE_NAME, value: ledger_session}
- {name: TAUTH_TENANT_ID, value: ledger}
- {name: TAUTH_URL, value: https://ledger-api.mprlab.com}
`)
	requireCurrentContractYAML(testingContext, "ledger-api.assets", service["assets"], `[{source: configs/config.ledger.yml, target: /srv/config.yml}]`)
	requireCurrentContractYAML(testingContext, "ledger-api.mounts", service["mounts"], `[{target: /srv/data, volume: data, read_only: false}]`)
	requireCurrentContractYAML(testingContext, "ledger-api.ports", service["ports"], `[{container_port: 50051, host_port: 50051, scope: private_lan}, {container_port: 8080, host_port: 18082, scope: private_lan}]`)
	requireCurrentContractYAML(testingContext, "ledger-api.readiness", service["readiness"], `endpoint: readiness`)
	requireCurrentContractYAML(testingContext, "ledger-api.endpoints", service["endpoints"], `
- {id: grpc, name: ledger.grpc, version: 1, endpoint: {scope: same_host, scheme: grpc, alias: ledger-api, port: 50051}, health: {protocol: tcp}}
- {id: grpc-private-lan, name: ledger.grpc.private-lan, version: 1, endpoint: {scope: private_lan, scheme: grpc, port: 50051}, health: {protocol: tcp}}
- {id: http, name: ledger.http, version: 1, endpoint: {scope: same_host, scheme: http, alias: ledger-http, port: 8080}, health: {protocol: http, path: /healthz, expected_status: 200}}
- {id: http-private-lan, name: ledger.http.private-lan, version: 1, endpoint: {scope: private_lan, scheme: http, port: 18082}, health: {protocol: http, path: /healthz, expected_status: 200}}
- {id: readiness, name: ledger.readiness.runtime.ledger-api, version: 1, endpoint: {scope: same_host, scheme: http, alias: runtime-ledger-api-readiness, port: 8080}, health: {protocol: http, path: /healthz, expected_status: 200}}
`)
	ingress := currentContractMap(testingContext, currentContractMap(testingContext, targets["ingress"])["caddy"])
	requireCurrentContractYAML(testingContext, "ingress.caddy.routes", ingress["routes"], `
- id: api
  hostname: ledger-api.mprlab.com
  listener: https
  tls: {mode: automatic}
  handlers:
  - {id: authentication, path_prefix: /auth, upstream: tauth.http}
  - {id: profile, path_prefix: /me, upstream: tauth.http}
  - {id: control-plane, path_prefix: /api, upstream: ledger.http}
  - {id: browser-config, path_prefix: /config-ui.yaml, upstream: ledger.http}
  - {id: health, path_prefix: /healthz, upstream: ledger.http}
`)
	requireCurrentContractYAML(testingContext, "targets.github_pages", targets["github_pages"], `sites: [{id: frontend, publication: frontend, url: https://ledger.mprlab.com/}]`)
	if _, err := os.Stat(filepath.Join(repositoryRoot, ".mprlab", "release.yml")); !os.IsNotExist(err) {
		testingContext.Fatalf("obsolete release policy file remains: %v", err)
	}
	runtimeConfiguration := string(readFile(testingContext, filepath.Join(repositoryRoot, "configs", "config.ledger.yml")))
	for _, requiredFragment := range []string{
		`jwt_issuer: "tauth"`,
		`login_path: "/auth/google"`,
		`logout_path: "/auth/logout"`,
		`nonce_path: "/auth/nonce"`,
		`session_path: "/auth/session"`,
	} {
		if !strings.Contains(runtimeConfiguration, requiredFragment) {
			testingContext.Fatalf("runtime configuration is missing canonical fragment %q", requiredFragment)
		}
	}
	for _, obsoleteVariable := range []string{
		"TAUTH_JWT_ISSUER",
		"TAUTH_LOGIN_PATH",
		"TAUTH_LOGOUT_PATH",
		"TAUTH_NONCE_PATH",
		"TAUTH_SESSION_PATH",
	} {
		if strings.Contains(runtimeConfiguration, obsoleteVariable) {
			testingContext.Fatalf("runtime configuration retains obsolete variable %q", obsoleteVariable)
		}
	}
	pagesConfiguration := string(readFile(testingContext, filepath.Join(repositoryRoot, "internal", "controlplane", "web", "config-ui.yaml")))
	for _, requiredFragment := range []string{
		`- "https://ledger.mprlab.com"`,
		`tauthUrl: "https://ledger-api.mprlab.com"`,
		`tenantId: "ledger"`,
		`providers:`,
		`clientId: "611549676198-d8800qv64voofseor1qod1euto5duivu.apps.googleusercontent.com"`,
		`loginPath: "/auth/google"`,
		`noncePath: "/auth/nonce"`,
		`apple:`,
		`password:`,
	} {
		if !strings.Contains(pagesConfiguration, requiredFragment) {
			testingContext.Fatalf("Pages authentication configuration is missing %q", requiredFragment)
		}
	}
	pagesTemplate := string(readFile(testingContext, filepath.Join(repositoryRoot, "internal", "controlplane", "web", "index.html")))
	for _, requiredFragment := range []string{
		`https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js`,
		`data-config-url="/config-ui.yaml"`,
		`data-mpr-ui-bundle-src="https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js"`,
	} {
		if !strings.Contains(pagesTemplate, requiredFragment) {
			testingContext.Fatalf("Pages template is missing %q", requiredFragment)
		}
	}
	pagesProfile := string(readFile(testingContext, filepath.Join(repositoryRoot, "internal", "controlplane", "web", "js", "profile.js")))
	for _, requiredFragment := range []string{
		`"http://localhost:8000": Object.freeze({ apiOrigin: "http://localhost:8000" })`,
		`"https://ledger.mprlab.com": Object.freeze({ apiOrigin: "https://ledger-api.mprlab.com" })`,
		`ledger_browser_profile_missing`,
	} {
		if !strings.Contains(pagesProfile, requiredFragment) {
			testingContext.Fatalf("browser profile is missing %q", requiredFragment)
		}
	}
	dockerfile := string(readFile(testingContext, filepath.Join(repositoryRoot, "Dockerfile")))
	for _, requiredFragment := range []string{
		"FROM scratch AS pages",
		"COPY internal/controlplane/web/config-ui.yaml /config-ui.yaml",
		"COPY internal/controlplane/web/js /assets/ledger/js",
	} {
		if !strings.Contains(dockerfile, requiredFragment) {
			testingContext.Fatalf("Dockerfile is missing Pages fragment %q", requiredFragment)
		}
	}

	requireExactIgnore(testingContext, filepath.Join(repositoryRoot, ".gitignore"), ".mprlab/deploy/.env", false)
	requireExactIgnore(testingContext, filepath.Join(repositoryRoot, "Dockerfile.dockerignore"), ".mprlab/deploy/.env", true)
}

func locateRepositoryRoot(testingContext *testing.T) string {
	testingContext.Helper()
	_, sourcePath, _, callerAvailable := runtime.Caller(0)
	if !callerAvailable {
		testingContext.Fatal("resolve lifecycle test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
}

func readFile(testingContext *testing.T, path string) []byte {
	testingContext.Helper()
	content, readError := os.ReadFile(path)
	if readError != nil {
		testingContext.Fatalf("read %s: %v", path, readError)
	}
	return content
}

func requireExactIgnore(testingContext *testing.T, path string, privatePath string, rejectNegations bool) {
	testingContext.Helper()
	lines := strings.Split(strings.ReplaceAll(string(readFile(testingContext, path)), "\r\n", "\n"), "\n")
	if !slices.Contains(lines, privatePath) {
		testingContext.Fatalf("%s must contain exact ignore %q", path, privatePath)
	}
	if rejectNegations {
		for _, line := range lines {
			if strings.HasPrefix(line, "!") {
				testingContext.Fatalf("%s must not contain Docker ignore negation %q", path, line)
			}
		}
	}
}

func sortedLifecycleStrings(values []string) []string {
	result := slices.Clone(values)
	slices.Sort(result)
	return result
}

func currentContractMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected native declaration mapping, got %#v", value)
	}
	return result
}

func currentManifestContract(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := yaml.Unmarshal(content, &envelope); err != nil {
		t.Fatalf("parse selected manifest: %v", err)
	}
	if len(envelope) != 1 {
		t.Fatalf("unexpected manifest envelope: %#v", envelope)
	}
	actual := currentContractMap(t, envelope["mprlab_resources"])
	keys := make([]string, 0, len(actual))
	for key := range actual {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	expected := []string{"artifacts", "environment", "lifecycle", "owner", "publications", "release", "targets"}
	if !reflect.DeepEqual(keys, expected) {
		t.Fatalf("selected manifest root keys=%v want=%v", keys, expected)
	}
	return actual
}

func currentContractDeclaration(t *testing.T, value any, identity string) map[string]any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected explicit declaration list, got %#v", value)
	}
	var found map[string]any
	for _, item := range items {
		declaration := currentContractMap(t, item)
		if declaration["id"] == identity {
			if found != nil {
				t.Fatalf("duplicate declaration %s", identity)
			}
			found = declaration
		}
	}
	if found == nil {
		t.Fatalf("missing native declaration %s", identity)
	}
	return found
}

func requireCurrentContractYAML(t *testing.T, path string, actual any, expectedText string) {
	t.Helper()
	var expected any
	if err := yaml.Unmarshal([]byte(expectedText), &expected); err != nil {
		t.Fatalf("parse expected %s: %v", path, err)
	}
	requireCurrentContractValue(t, path, actual, expected)
}

func requireCurrentContractValue(t *testing.T, path string, actual, expected any) {
	t.Helper()
	switch wanted := expected.(type) {
	case map[string]any:
		mapping, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s requires a mapping, got %#v", path, actual)
		}
		if !strings.HasSuffix(path, ".assets[]") && len(mapping) != len(wanted) {
			t.Fatalf("%s has unexpected fields: %#v", path, actual)
		}
		for key, value := range wanted {
			observed, exists := mapping[key]
			if !exists {
				t.Fatalf("%s lacks %s", path, key)
			}
			requireCurrentContractValue(t, path+"."+key, observed, value)
		}
	case []any:
		items, ok := actual.([]any)
		if !ok || len(items) != len(wanted) {
			t.Fatalf("%s list=%#v want=%#v", path, actual, wanted)
		}
		for index, value := range wanted {
			observed := items[index]
			if declaration, isMapping := value.(map[string]any); isMapping {
				identityKey := "id"
				if _, exists := declaration[identityKey]; !exists {
					identityKey = "name"
				}
				if identity, exists := declaration[identityKey]; exists {
					matches := 0
					for _, candidate := range items {
						if entry, valid := candidate.(map[string]any); valid && entry[identityKey] == identity {
							observed = candidate
							matches++
						}
					}
					if matches != 1 {
						t.Fatalf("%s requires one %s=%v, got %d", path, identityKey, identity, matches)
					}
				}
			}
			requireCurrentContractValue(t, path+"[]", observed, value)
		}
	default:
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%s=%#v want=%#v", path, actual, expected)
		}
	}
}
