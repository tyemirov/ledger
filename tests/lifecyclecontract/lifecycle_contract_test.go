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
	SchemaVersion int                 `yaml:"schema_version"`
	Owner         string              `yaml:"owner"`
	Resources     []lifecycleResource `yaml:"resources"`
}

type lifecycleResource struct {
	Kind            string             `yaml:"kind"`
	ID              string             `yaml:"id"`
	Bindings        map[string]string  `yaml:"bindings"`
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
}

type retiredService struct {
	Project string `yaml:"project"`
	Service string `yaml:"service"`
}

type containerImage struct {
	ID         string         `yaml:"id"`
	Repository string         `yaml:"repository"`
	Visibility string         `yaml:"visibility"`
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
}

type servicePlacement struct {
	Group       string `yaml:"group"`
	Cardinality string `yaml:"cardinality"`
}

type environmentBinding struct {
	Resource string `yaml:"resource"`
	Output   string `yaml:"output"`
	Secret   string `yaml:"secret"`
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
	Protocol string `yaml:"protocol"`
}

func TestSchemaV3LifecycleContract(testingContext *testing.T) {
	testingContext.Parallel()
	repositoryRoot := locateRepositoryRoot(testingContext)
	manifestPath := filepath.Join(repositoryRoot, ".mprlab", "deploy", "resources.yml")
	manifestBytes := readFile(testingContext, manifestPath)

	var documentNode yaml.Node
	if unmarshalError := yaml.Unmarshal(manifestBytes, &documentNode); unmarshalError != nil {
		testingContext.Fatalf("parse deployment manifest: %v", unmarshalError)
	}
	requireMappingKeys(testingContext, documentNode.Content[0], []string{"mprlab_resources"})
	requireMappingKeys(
		testingContext,
		mappingValue(testingContext, documentNode.Content[0], "mprlab_resources"),
		[]string{"owner", "resources", "schema_version"},
	)

	var envelope manifestEnvelope
	if unmarshalError := yaml.Unmarshal(manifestBytes, &envelope); unmarshalError != nil {
		testingContext.Fatalf("decode deployment manifest: %v", unmarshalError)
	}
	manifest := envelope.MPRLabResources
	if manifest.SchemaVersion != 3 || manifest.Owner != "ledger" {
		testingContext.Fatalf("unexpected manifest identity: schema=%d owner=%q", manifest.SchemaVersion, manifest.Owner)
	}
	if len(manifest.Resources) != 3 {
		testingContext.Fatalf("expected three production resources, got %d", len(manifest.Resources))
	}

	privateResource := requireResource(testingContext, manifest.Resources, "private_values", "private")
	expectedBindings := map[string]string{
		"database-url":                "DATABASE_URL",
		"hecate-tenant-secret":        "HECATE_TENANT_SECRET",
		"namesignal-tenant-secret":    "NAMESIGNAL_TENANT_SECRET",
		"poodlescanner-tenant-secret": "POODLESCANNER_TENANT_SECRET",
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
	if ledgerImage.ID != "ledger-image" || ledgerImage.Repository != "ghcr.io/tyemirov/ledger" || ledgerImage.Visibility != "public" {
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
		"DATABASE_URL":                {Resource: "private", Output: "database-url"},
		"HECATE_TENANT_SECRET":        {Resource: "private", Output: "hecate-tenant-secret"},
		"NAMESIGNAL_TENANT_SECRET":    {Resource: "private", Output: "namesignal-tenant-secret"},
		"POODLESCANNER_TENANT_SECRET": {Resource: "private", Output: "poodlescanner-tenant-secret"},
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
	if !reflect.DeepEqual(ledgerService.Ports, []servicePort{{ContainerPort: 50051}}) {
		testingContext.Fatalf("unexpected service ports: %#v", ledgerService.Ports)
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

	requireExactIgnore(testingContext, filepath.Join(repositoryRoot, ".gitignore"), ".mprlab/deploy/.env", false)
	requireExactIgnore(testingContext, filepath.Join(repositoryRoot, "Dockerfile.dockerignore"), ".mprlab/deploy/.env", true)

	makefileContent := string(readFile(testingContext, filepath.Join(repositoryRoot, "Makefile")))
	for _, requiredFragment := range []string{
		"release publish deploy:",
		`gateway_root="$$(dirname "$${application_root}")/mprlab-gateway"`,
		"\"app-$@\"",
		"MPRLAB_APP_ROOT",
	} {
		if !strings.Contains(makefileContent, requiredFragment) {
			testingContext.Fatalf("Makefile is missing lifecycle wrapper fragment %q", requiredFragment)
		}
	}
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
