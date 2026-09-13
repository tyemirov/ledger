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

type manifestEnvelope struct {
	MPRLabResources applicationManifest `yaml:"mprlab_resources"`
}

type applicationManifest struct {
	Owner     string              `yaml:"owner"`
	Release   releasePolicy       `yaml:"release"`
	Resources []lifecycleResource `yaml:"resources"`
}

type releasePolicy struct {
	Scheme string `yaml:"scheme"`
}

type lifecycleResource struct {
	Kind            string             `yaml:"kind"`
	ID              string             `yaml:"id"`
	Bindings        map[string]string  `yaml:"bindings"`
	Capability      string             `yaml:"capability"`
	Placement       *servicePlacement  `yaml:"placement"`
	Profiles        *[]string          `yaml:"profiles"`
	RetiredServices []retiredService   `yaml:"retired_services"`
	Images          []containerImage   `yaml:"images"`
	Services        []composeService   `yaml:"services"`
	Volumes         []retainedVolume   `yaml:"volumes"`
	Name            string             `yaml:"name"`
	Version         int                `yaml:"version"`
	Project         string             `yaml:"project"`
	Service         string             `yaml:"service"`
	Endpoint        capabilityEndpoint `yaml:"endpoint"`
	Health          capabilityHealth   `yaml:"health"`
	Tenant          tauthTenant        `yaml:"tenant"`
	Hostname        string             `yaml:"hostname"`
	Listener        string             `yaml:"listener"`
	Handlers        []routeHandler     `yaml:"handlers"`
	TLS             routeTLS           `yaml:"tls"`
	Protocol        string             `yaml:"protocol"`
	URL             string             `yaml:"url"`
	ExpectedStatus  int                `yaml:"expected_status"`
	Repository      string             `yaml:"repository"`
	Branch          string             `yaml:"branch"`
	Domain          string             `yaml:"domain"`
	Source          pagesSource        `yaml:"source"`
	Verification    pagesVerification  `yaml:"verification"`
}

type outputReference struct {
	Resource string `yaml:"resource"`
	Output   string `yaml:"output"`
}

type tauthTenant struct {
	ID                string          `yaml:"id"`
	DisplayName       string          `yaml:"display_name"`
	Origins           []string        `yaml:"origins"`
	GoogleWebClientID outputReference `yaml:"google_web_client_id"`
	JWTSigningKey     outputReference `yaml:"jwt_signing_key"`
	Cookie            tauthCookie     `yaml:"cookie"`
}

type tauthCookie struct {
	Domain      string `yaml:"domain"`
	SessionName string `yaml:"session_name"`
	RefreshName string `yaml:"refresh_name"`
}

type routeHandler struct {
	ID         string `yaml:"id"`
	PathPrefix string `yaml:"path_prefix"`
	Upstream   string `yaml:"upstream"`
}

type routeTLS struct {
	Mode string `yaml:"mode"`
}

type pagesSource struct {
	Kind       string `yaml:"kind"`
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile"`
	Target     string `yaml:"target"`
}

type pagesVerification struct {
	Path string `yaml:"path"`
}

type retiredService struct {
	Project string `yaml:"project"`
	Service string `yaml:"service"`
}

type containerImage struct {
	ID         string         `yaml:"id"`
	Repository string         `yaml:"repository"`
	Build      containerBuild `yaml:"build"`
}

type containerBuild struct {
	Context    string   `yaml:"context"`
	Dockerfile string   `yaml:"dockerfile"`
	Platforms  []string `yaml:"platforms"`
}

type composeService struct {
	ID          string                        `yaml:"id"`
	Image       string                        `yaml:"image"`
	Placement   servicePlacement              `yaml:"placement"`
	Environment map[string]environmentBinding `yaml:"environment"`
	Assets      []runtimeAsset                `yaml:"assets"`
	Mounts      []volumeMount                 `yaml:"mounts"`
	Ports       []servicePort                 `yaml:"ports"`
	Readiness   serviceReadiness              `yaml:"readiness"`
}

type serviceReadiness struct {
	Protocol       string `yaml:"protocol"`
	Port           int    `yaml:"port"`
	Path           string `yaml:"path"`
	ExpectedStatus int    `yaml:"expected_status"`
}

type servicePlacement struct {
	Group       string `yaml:"group"`
	Cardinality string `yaml:"cardinality"`
}

type environmentBinding struct {
	Resource string `yaml:"resource"`
	Output   string `yaml:"output"`
	Secret   string `yaml:"secret"`
	Value    string `yaml:"value"`
}

type runtimeAsset struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	Mode   string `yaml:"mode"`
}

type volumeMount struct {
	Volume   string `yaml:"volume"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

type servicePort struct {
	ContainerPort int `yaml:"container_port"`
}

type retainedVolume struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Retention string `yaml:"retention"`
}

type capabilityEndpoint struct {
	Scope  string `yaml:"scope"`
	Scheme string `yaml:"scheme"`
	Alias  string `yaml:"alias"`
	Port   int    `yaml:"port"`
}

type capabilityHealth struct {
	Protocol       string `yaml:"protocol"`
	Path           string `yaml:"path"`
	ExpectedStatus int    `yaml:"expected_status"`
}

func TestVersionlessLifecycleContract(testingContext *testing.T) {
	testingContext.Parallel()
	repositoryRoot := locateRepositoryRoot(testingContext)
	manifestPath := filepath.Join(repositoryRoot, ".mprlab", "deploy", "resources.yml")
	manifestBytes := readFile(testingContext, manifestPath)
	if strings.Contains(string(manifestBytes), "\n          visibility:") {
		testingContext.Fatal("application image retains removed visibility field")
	}

	var documentNode yaml.Node
	if unmarshalError := yaml.Unmarshal(manifestBytes, &documentNode); unmarshalError != nil {
		testingContext.Fatalf("parse deployment manifest: %v", unmarshalError)
	}
	requireMappingKeys(testingContext, documentNode.Content[0], []string{"mprlab_resources"})
	requireMappingKeys(
		testingContext,
		mappingValue(testingContext, documentNode.Content[0], "mprlab_resources"),
		[]string{"owner", "release", "resources"},
	)
	requireMappingKeys(
		testingContext,
		mappingValue(
			testingContext,
			mappingValue(testingContext, documentNode.Content[0], "mprlab_resources"),
			"release",
		),
		[]string{"scheme"},
	)

	var envelope manifestEnvelope
	if unmarshalError := yaml.Unmarshal(manifestBytes, &envelope); unmarshalError != nil {
		testingContext.Fatalf("decode deployment manifest: %v", unmarshalError)
	}
	manifest := envelope.MPRLabResources
	if manifest.Owner != "ledger" {
		testingContext.Fatalf("unexpected manifest owner: %q", manifest.Owner)
	}
	if manifest.Release != (releasePolicy{Scheme: "semver"}) {
		testingContext.Fatalf("unexpected release policy: %#v", manifest.Release)
	}
	if _, statError := os.Stat(filepath.Join(repositoryRoot, ".mprlab", "release.yml")); !os.IsNotExist(statError) {
		testingContext.Fatalf("obsolete release policy file remains: %v", statError)
	}
	if len(manifest.Resources) != 8 {
		testingContext.Fatalf("expected eight production resources, got %d", len(manifest.Resources))
	}

	privateResource := requireResource(testingContext, manifest.Resources, "private_values", "private")
	expectedBindings := map[string]string{
		"database-url":           "DATABASE_URL",
		"tauth-jwt-signing-key":  "TAUTH_JWT_SIGNING_KEY",
		"tauth-google-client-id": "TAUTH_GOOGLE_CLIENT_ID",
	}
	if !reflect.DeepEqual(privateResource.Bindings, expectedBindings) {
		testingContext.Fatalf("unexpected private bindings: %#v", privateResource.Bindings)
	}

	composeResource := requireResource(testingContext, manifest.Resources, "compose_project", "runtime")
	if composeResource.Placement != nil || composeResource.Profiles != nil {
		testingContext.Fatal("Compose placement must exist only on services and profiles must be absent")
	}
	if !reflect.DeepEqual(composeResource.RetiredServices, []retiredService{{Project: "mprlab-nginx-gateway", Service: "ledger-api"}}) {
		testingContext.Fatalf("unexpected retired services: %#v", composeResource.RetiredServices)
	}
	if len(composeResource.Images) != 1 {
		testingContext.Fatalf("expected one image, got %d", len(composeResource.Images))
	}
	ledgerImage := composeResource.Images[0]
	if ledgerImage.ID != "ledger-image" || ledgerImage.Repository != "ghcr.io/tyemirov/ledger" {
		testingContext.Fatalf("unexpected image declaration: %#v", ledgerImage)
	}
	if ledgerImage.Build.Context != "." || ledgerImage.Build.Dockerfile != "Dockerfile" || !reflect.DeepEqual(ledgerImage.Build.Platforms, []string{"linux/amd64", "linux/arm64"}) {
		testingContext.Fatalf("unexpected image build declaration: %#v", ledgerImage.Build)
	}
	if len(composeResource.Services) != 1 {
		testingContext.Fatalf("expected one service, got %d", len(composeResource.Services))
	}
	ledgerService := composeResource.Services[0]
	if ledgerService.ID != "ledger-api" || ledgerService.Image != "ledger-image" || ledgerService.Placement != (servicePlacement{Group: "gateway", Cardinality: "one"}) {
		testingContext.Fatalf("unexpected Ledger service declaration: %#v", ledgerService)
	}
	expectedEnvironment := map[string]environmentBinding{
		"DATABASE_URL":              {Resource: "private", Output: "database-url"},
		"LEDGER_PUBLIC_ORIGIN":      {Value: "https://ledger.mprlab.com"},
		"TAUTH_JWT_SIGNING_KEY":     {Resource: "authentication", Output: "jwt-signing-key"},
		"TAUTH_GOOGLE_CLIENT_ID":    {Resource: "authentication", Output: "google-web-client-id"},
		"TAUTH_SESSION_COOKIE_NAME": {Resource: "authentication", Output: "session-cookie-name"},
		"TAUTH_TENANT_ID":           {Resource: "authentication", Output: "tenant-id"},
		"TAUTH_URL":                 {Value: "https://ledger-api.mprlab.com"},
	}
	if !reflect.DeepEqual(ledgerService.Environment, expectedEnvironment) {
		testingContext.Fatalf("unexpected service environment: %#v", ledgerService.Environment)
	}
	if !reflect.DeepEqual(ledgerService.Assets, []runtimeAsset{{Source: "configs/config.ledger.yml", Target: "/srv/config.yml", Mode: "0444"}}) {
		testingContext.Fatalf("unexpected runtime assets: %#v", ledgerService.Assets)
	}
	if !reflect.DeepEqual(ledgerService.Mounts, []volumeMount{{Volume: "data", Target: "/srv/data", ReadOnly: false}}) {
		testingContext.Fatalf("unexpected volume mounts: %#v", ledgerService.Mounts)
	}
	if !reflect.DeepEqual(ledgerService.Ports, []servicePort{{ContainerPort: 50051}, {ContainerPort: 8080}}) {
		testingContext.Fatalf("unexpected service ports: %#v", ledgerService.Ports)
	}
	if ledgerService.Readiness != (serviceReadiness{Protocol: "http", Port: 8080, Path: "/healthz", ExpectedStatus: 200}) {
		testingContext.Fatalf("unexpected service readiness: %#v", ledgerService.Readiness)
	}
	if !reflect.DeepEqual(composeResource.Volumes, []retainedVolume{{ID: "data", Name: "ledger-data", Retention: "retain"}}) {
		testingContext.Fatalf("unexpected retained volumes: %#v", composeResource.Volumes)
	}

	capabilityResource := requireResource(testingContext, manifest.Resources, "runtime_capability", "grpc")
	if capabilityResource.Name != "ledger.grpc" || capabilityResource.Version != 1 || capabilityResource.Project != "runtime" || capabilityResource.Service != "ledger-api" {
		testingContext.Fatalf("unexpected runtime capability: %#v", capabilityResource)
	}
	if capabilityResource.Endpoint != (capabilityEndpoint{Scope: "same_host", Scheme: "grpc", Alias: "ledger-api", Port: 50051}) || capabilityResource.Health != (capabilityHealth{Protocol: "tcp"}) {
		testingContext.Fatalf("unexpected capability endpoint or health: %#v %#v", capabilityResource.Endpoint, capabilityResource.Health)
	}
	httpCapability := requireResource(testingContext, manifest.Resources, "runtime_capability", "http")
	if httpCapability.Name != "ledger.http" || httpCapability.Version != 1 || httpCapability.Project != "runtime" || httpCapability.Service != "ledger-api" {
		testingContext.Fatalf("unexpected HTTP runtime capability: %#v", httpCapability)
	}
	if httpCapability.Endpoint != (capabilityEndpoint{Scope: "same_host", Scheme: "http", Alias: "ledger-http", Port: 8080}) || httpCapability.Health != (capabilityHealth{Protocol: "http", Path: "/healthz", ExpectedStatus: 200}) {
		testingContext.Fatalf("unexpected HTTP capability endpoint or health: %#v %#v", httpCapability.Endpoint, httpCapability.Health)
	}

	tauthResource := requireResource(testingContext, manifest.Resources, "tauth_tenant", "authentication")
	if tauthResource.Capability != "tauth.tenants" || tauthResource.Version != 1 {
		testingContext.Fatalf("unexpected TAuth tenant capability: %#v", tauthResource)
	}
	expectedTAuthTenant := tauthTenant{
		ID:                "ledger",
		DisplayName:       "Ledger",
		Origins:           []string{"https://ledger.mprlab.com"},
		GoogleWebClientID: outputReference{Resource: "private", Output: "tauth-google-client-id"},
		JWTSigningKey:     outputReference{Resource: "private", Output: "tauth-jwt-signing-key"},
		Cookie:            tauthCookie{Domain: "ledger-api.mprlab.com", SessionName: "ledger_session", RefreshName: "ledger_refresh"},
	}
	if !reflect.DeepEqual(tauthResource.Tenant, expectedTAuthTenant) {
		testingContext.Fatalf("unexpected TAuth tenant: %#v", tauthResource.Tenant)
	}

	apiRoute := requireResource(testingContext, manifest.Resources, "caddy_route", "api")
	if apiRoute.Hostname != "ledger-api.mprlab.com" || apiRoute.Listener != "https" || apiRoute.TLS != (routeTLS{Mode: "automatic"}) {
		testingContext.Fatalf("unexpected API route: %#v", apiRoute)
	}
	expectedHandlers := []routeHandler{
		{ID: "authentication", PathPrefix: "/auth", Upstream: "tauth.http"},
		{ID: "profile", PathPrefix: "/me", Upstream: "tauth.http"},
		{ID: "control-plane", PathPrefix: "/api", Upstream: "ledger.http"},
		{ID: "browser-config", PathPrefix: "/config-ui.yaml", Upstream: "ledger.http"},
		{ID: "health", PathPrefix: "/healthz", Upstream: "ledger.http"},
	}
	if !reflect.DeepEqual(apiRoute.Handlers, expectedHandlers) {
		testingContext.Fatalf("unexpected API route handlers: %#v", apiRoute.Handlers)
	}

	apiHealth := requireResource(testingContext, manifest.Resources, "health_check", "api-health")
	if apiHealth.Protocol != "http" || apiHealth.URL != "https://ledger-api.mprlab.com/healthz" || apiHealth.ExpectedStatus != 200 {
		testingContext.Fatalf("unexpected API health check: %#v", apiHealth)
	}

	frontend := requireResource(testingContext, manifest.Resources, "github_pages", "frontend")
	if frontend.Repository != "tyemirov/ledger" || frontend.Branch != "gh-pages" || frontend.Domain != "ledger.mprlab.com" || frontend.URL != "https://ledger.mprlab.com/" {
		testingContext.Fatalf("unexpected frontend resource: %#v", frontend)
	}
	if frontend.Source != (pagesSource{Kind: "container", Context: ".", Dockerfile: "Dockerfile", Target: "pages"}) || frontend.Verification != (pagesVerification{Path: "/.mprlab-release.json"}) {
		testingContext.Fatalf("unexpected frontend source or verification: %#v %#v", frontend.Source, frontend.Verification)
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

func requireResource(testingContext *testing.T, resources []lifecycleResource, kind string, id string) lifecycleResource {
	testingContext.Helper()
	for _, resource := range resources {
		if resource.Kind == kind && resource.ID == id {
			return resource
		}
	}
	testingContext.Fatalf("missing resource kind=%s id=%s", kind, id)
	return lifecycleResource{}
}

func requireMappingKeys(testingContext *testing.T, mappingNode *yaml.Node, expectedKeys []string) {
	testingContext.Helper()
	if mappingNode.Kind != yaml.MappingNode {
		testingContext.Fatalf("expected YAML mapping node, got %d", mappingNode.Kind)
	}
	actualKeys := make([]string, 0, len(mappingNode.Content)/2)
	for contentIndex := 0; contentIndex < len(mappingNode.Content); contentIndex += 2 {
		actualKeys = append(actualKeys, mappingNode.Content[contentIndex].Value)
	}
	slices.Sort(actualKeys)
	if !reflect.DeepEqual(actualKeys, expectedKeys) {
		testingContext.Fatalf("unexpected mapping keys: got %v want %v", actualKeys, expectedKeys)
	}
}

func mappingValue(testingContext *testing.T, mappingNode *yaml.Node, key string) *yaml.Node {
	testingContext.Helper()
	for contentIndex := 0; contentIndex < len(mappingNode.Content); contentIndex += 2 {
		if mappingNode.Content[contentIndex].Value == key {
			return mappingNode.Content[contentIndex+1]
		}
	}
	testingContext.Fatalf("missing YAML mapping key %q", key)
	return nil
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
