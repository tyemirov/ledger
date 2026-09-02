package controlplanecontract

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPIContract(test *testing.T) {
	raw, err := os.ReadFile("../../api/control/v1/openapi.yaml")
	if err != nil {
		test.Fatalf("read OpenAPI: %v", err)
	}
	var document struct {
		OpenAPI string                          `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		test.Fatalf("parse OpenAPI: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		test.Fatalf("OpenAPI version is %q", document.OpenAPI)
	}
	want := map[string][]string{
		"/healthz":                             {"get"},
		"/api/user-account":                    {"get", "put"},
		"/api/tenants":                         {"get", "post"},
		"/api/tenants/{tenant_id}":             {"get"},
		"/api/tenants/{tenant_id}/credentials": {"get", "post"},
		"/api/tenants/{tenant_id}/credentials/{credential_id}": {"delete"},
	}
	if len(document.Paths) != len(want) {
		test.Fatalf("OpenAPI path count is %d, want %d", len(document.Paths), len(want))
	}
	for path, methods := range want {
		operations, ok := document.Paths[path]
		if !ok {
			test.Fatalf("OpenAPI path %q is missing", path)
		}
		for _, method := range methods {
			if _, ok := operations[method]; !ok {
				test.Fatalf("OpenAPI operation %s %s is missing", method, path)
			}
		}
	}
}
