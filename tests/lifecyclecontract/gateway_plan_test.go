package lifecyclecontract_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGatewayPlanOwnsGitTransport(testingContext *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	repository := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	fixture := testingContext.TempDir()
	gatewaySource := filepath.Join(fixture, "gateway.c")
	gateway := filepath.Join(fixture, "gateway")
	if err := writeFixture(gatewaySource, `#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
int main(int argc, char **argv) {
 if (argc != 4 || strcmp(argv[1], "app-plan-release") || strcmp(argv[2], "--app-root")) return 91;
 const char *private_inputs[] = {
  "DATABASE_URL", "DEPLOY_SUDO_PASSWORD_TUTOSH", "MPRLAB_TAUTH_MANAGEMENT_URL",
  "MPRLAB_TAUTH_PROVISIONING_CREDENTIALS", "TAUTH_GOOGLE_CLIENT_ID", "TAUTH_JWT_SIGNING_KEY"
 };
 for (size_t index = 0; index < sizeof(private_inputs) / sizeof(private_inputs[0]); index++) {
  if (getenv(private_inputs[index]) != NULL) return 94;
 }
 char path[4096];
 if (snprintf(path, sizeof(path), "%s/.mprlab/deploy/.env", argv[3]) >= (int)sizeof(path)) return 95;
 FILE *environment = fopen(path, "r");
 if (environment == NULL) return 96;
 char content[4096] = {0};
 size_t length = fread(content, 1, sizeof(content) - 1, environment);
 if (ferror(environment) || fclose(environment) || length == 0) return 97;
 const char *bindings[] = {
  "DATABASE_URL=sqlite:///srv/data/ledger.db\n",
  "DEPLOY_SUDO_PASSWORD_TUTOSH=fixture-only-password\n",
  "MPRLAB_TAUTH_MANAGEMENT_URL=https://tauth.example.invalid\n",
  "MPRLAB_TAUTH_PROVISIONING_CREDENTIALS=fixture-only-credential\n",
  "TAUTH_GOOGLE_CLIENT_ID=fixture-only-client\n",
  "TAUTH_JWT_SIGNING_KEY=fixture-only-signing-key\n"
 };
 for (size_t index = 0; index < sizeof(bindings) / sizeof(bindings[0]); index++) {
  if (strstr(content, bindings[index]) == NULL) return 98;
 }
 const char *operator_root = getenv("MPRLAB_GATEWAY_OPERATOR_ROOT");
 if (operator_root == NULL || strstr(operator_root, "/operator") == NULL) return 99;
 execlp("git", "git", "-C", argv[3], "ls-remote", "origin", (char *)0);
 return 92;
}
`); err != nil {
		testingContext.Fatal(err)
	}
	// The compiler creates the external command; the test does not change permissions.
	build := exec.Command("cc", gatewaySource, "-o", gateway)
	if output, err := build.CombinedOutput(); err != nil {
		testingContext.Fatalf("compile Gateway boundary: %v: %s", err, output)
	}
	trap := filepath.Join(fixture, "transport-trap.sh")
	if err := writeFixture(trap, "printf intercepted > \"$LEDGER_PLAN_TRAP_MARKER\"\nexit 93\n"); err != nil {
		testingContext.Fatal(err)
	}
	globalConfig := filepath.Join(fixture, "global.gitconfig")
	rewrite := "ext::sh " + trap
	if err := writeFixture(globalConfig, "[url \""+rewrite+"\"]\n insteadOf = git@github.com:tyemirov/ledger.git\n[protocol \"ext\"]\n allow = always\n"); err != nil {
		testingContext.Fatal(err)
	}
	for _, scenario := range []struct {
		name        string
		environment []string
	}{
		{"ssh-command", []string{"GIT_SSH_COMMAND=sh '" + trap + "'", "GIT_SSH_VARIANT=simple"}},
		{"environment-config", []string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=url." + rewrite + ".insteadOf", "GIT_CONFIG_VALUE_0=git@github.com:tyemirov/ledger.git", "GIT_CONFIG_KEY_1=protocol.ext.allow", "GIT_CONFIG_VALUE_1=always"}},
		{"global-config", []string{"GIT_CONFIG_GLOBAL=" + globalConfig}},
		{"private-inputs", []string{
			"DATABASE_URL=ambient-database",
			"DEPLOY_SUDO_PASSWORD_TUTOSH=ambient-password",
			"MPRLAB_TAUTH_MANAGEMENT_URL=ambient-management",
			"MPRLAB_TAUTH_PROVISIONING_CREDENTIALS=ambient-credential",
			"TAUTH_GOOGLE_CLIENT_ID=ambient-client",
			"TAUTH_JWT_SIGNING_KEY=ambient-key",
			"MPRLAB_GATEWAY_OPERATOR_ROOT=ambient-operator",
		}},
	} {
		testingContext.Run(scenario.name, func(testingContext *testing.T) {
			marker := filepath.Join(testingContext.TempDir(), "transport-used")
			command := exec.Command("bash", filepath.Join(repository, "tests/gateway-plan.sh"))
			command.Dir = repository
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "GIT_") {
					command.Env = append(command.Env, value)
				}
			}
			command.Env = append(command.Env, "CI=1", "MPRLAB_GATEWAY_EXECUTABLE="+gateway, "LEDGER_PLAN_TRAP_MARKER="+marker)
			command.Env = append(command.Env, scenario.environment...)
			output, err := command.CombinedOutput()
			if _, markerErr := os.Stat(marker); !os.IsNotExist(markerErr) {
				testingContext.Fatal("inherited transport intercepted fixture")
			}
			if err != nil {
				testingContext.Fatalf("planner fixture failed: %v: %s", err, output)
			}
		})
	}
}

func writeFixture(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
