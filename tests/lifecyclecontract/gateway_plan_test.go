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
	if err := writeFixture(gatewaySource, `#include <string.h>
#include <unistd.h>
int main(int argc, char **argv) {
 if (argc != 4 || strcmp(argv[1], "app-plan-release") || strcmp(argv[2], "--app-root")) return 91;
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
