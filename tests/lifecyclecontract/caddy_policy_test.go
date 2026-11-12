package lifecyclecontract_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

type executionPolicy struct {
	Timeouts map[string]string `yaml:"timeouts"`
	Polling  struct {
		Initial    string `yaml:"initial"`
		Maximum    string `yaml:"maximum"`
		Increment  string `yaml:"increment"`
		Multiplier int    `yaml:"multiplier"`
		Attempts   int    `yaml:"attempt_limit"`
	} `yaml:"polling"`
}

func TestAPIRouteUsesSharedCaddyExecutionPolicy(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	read := func(path string, value any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}
	}
	var provider struct {
		Execution executionPolicy `yaml:"execution"`
	}
	read(filepath.Join(root, "tests/lifecyclecontract/testdata/shared-caddy-policy.yml"), &provider)
	var manifest struct {
		Resources struct {
			Targets struct {
				Ingress struct {
					Caddy struct {
						Routes []struct {
							ID         string `yaml:"id"`
							Operations []struct {
								Name            string `yaml:"name"`
								executionPolicy `yaml:",inline"`
							} `yaml:"operations"`
						} `yaml:"routes"`
					} `yaml:"caddy"`
				} `yaml:"ingress"`
			} `yaml:"targets"`
		} `yaml:"mprlab_resources"`
	}
	read(filepath.Join(root, ".mprlab/deploy/resources.yml"), &manifest)
	normalize := func(policy executionPolicy) executionPolicy {
		t.Helper()
		duration := func(value string) string {
			t.Helper()
			parsed, err := time.ParseDuration(value)
			if err != nil {
				t.Fatal(err)
			}
			return parsed.String()
		}
		for name, value := range policy.Timeouts {
			policy.Timeouts[name] = duration(value)
		}
		policy.Polling.Initial = duration(policy.Polling.Initial)
		policy.Polling.Maximum = duration(policy.Polling.Maximum)
		policy.Polling.Increment = duration(policy.Polling.Increment)
		return policy
	}
	for _, route := range manifest.Resources.Targets.Ingress.Caddy.Routes {
		if route.ID != "api" {
			continue
		}
		for _, operation := range route.Operations {
			if operation.Name != "app-deploy" {
				continue
			}
			if !reflect.DeepEqual(normalize(operation.executionPolicy), normalize(provider.Execution)) {
				t.Fatal("API route deployment policy differs from the published shared Caddy contract")
			}
			return
		}
		t.Fatal("API route must declare the shared Caddy deployment policy without inheriting application runtime settings")
	}
	t.Fatal("API route is absent")
}
