#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT
application_root="$fixture_root/application with spaces"
fixture_bin="$fixture_root/tool commands"
make_command="$(command -v make)"
mkdir -p "$application_root/scripts" "$fixture_bin"
cp "$repository_root/Makefile" "$application_root/Makefile"
if [ -f "$repository_root/scripts/ensure-go-analyzers.sh" ]; then
  cp "$repository_root/scripts/ensure-go-analyzers.sh" "$application_root/scripts/ensure-go-analyzers.sh"
fi
ln -s "$(command -v bash)" "$fixture_bin/bash"

# Only the external Go command is controlled. Make and the bootstrap helper are real.
cat > "$fixture_root/go.c" <<'C'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char **argv) {
    const char *active = getenv("TEST_ACTIVE_VERSION");
    if (argc == 3 && strcmp(argv[1], "env") == 0 && strcmp(argv[2], "GOVERSION") == 0) {
        if (getenv("TEST_ACTIVE_FAILURE")) return 41;
        puts(active);
        return 0;
    }
    if (argc == 4 && strcmp(argv[1], "version") == 0 && strcmp(argv[2], "-m") == 0) {
        if (getenv("TEST_METADATA_FAILURE")) return 43;
        char file[8192], version[256];
        snprintf(file, sizeof(file), "%s.goversion", argv[3]);
        FILE *input = fopen(file, "r");
        if (!input || !fgets(version, sizeof(version), input)) return 44;
        fclose(input);
        version[strcspn(version, "\r\n")] = 0;
        printf("%s: %s\n", argv[3], version);
        return 0;
    }
    if (argc == 3 && strcmp(argv[1], "install") == 0) {
        char module[4096], tool[256], target[8192];
        snprintf(module, sizeof(module), "%s", argv[2]);
        char *at = strrchr(module, '@');
        if (!at) return 45;
        *at = 0;
        snprintf(tool, sizeof(tool), "%s", strrchr(module, '/') + 1);
        FILE *log = fopen(getenv("TEST_INSTALL_LOG"), "a");
        if (!log) return 46;
        fprintf(log, "%s\n", argv[2]);
        fclose(log);
        const char *failure = getenv("TEST_INSTALL_FAILURE");
        if (failure && strcmp(failure, tool) == 0) return 42;
        if (getenv("TEST_INSTALL_INEFFECTIVE")) return 0;
        snprintf(target, sizeof(target), "%s/%s", getenv("TEST_FIXTURE_BIN"), tool);
        if (access(target, F_OK) != 0 && symlink("/usr/bin/true", target) != 0) return 47;
        strncat(target, ".goversion", sizeof(target) - strlen(target) - 1);
        FILE *output = fopen(target, "w");
        if (!output) return 48;
        fprintf(output, "%s\n", active);
        fclose(output);
        return 0;
    }
    fprintf(stderr, "unexpected controlled Go invocation\n");
    return 49;
}
C
cc "$fixture_root/go.c" -o "$fixture_bin/go"
unset MAKEFLAGS MFLAGS MAKEOVERRIDES TEST_INSTALL_FAILURE TEST_INSTALL_INEFFECTIVE TEST_METADATA_FAILURE TEST_ACTIVE_FAILURE
export TEST_ACTIVE_VERSION=go1.27.1
export TEST_FIXTURE_BIN="$fixture_bin"
export TEST_INSTALL_LOG="$fixture_root/install.log"
tools=(staticcheck ineffassign errcheck deadcode)
modules=(honnef.co/go/tools/cmd/staticcheck@latest github.com/gordonklaus/ineffassign@latest github.com/kisielk/errcheck@latest golang.org/x/tools/cmd/deadcode@latest)

reset_fixture() {
  : > "$TEST_INSTALL_LOG"
  for tool in "${tools[@]}"; do
    rm -f "$fixture_bin/$tool" "$fixture_bin/$tool.goversion"
    ln -s /usr/bin/true "$fixture_bin/$tool"
    printf '%s\n' "$TEST_ACTIVE_VERSION" > "$fixture_bin/$tool.goversion"
  done
}
run_make() {
  actual_status=0
  PATH="$fixture_bin" "$make_command" --no-print-directory -s -C "$application_root" tools > "$fixture_root/output.log" 2>&1 || actual_status=$?
  if [ "$actual_status" -ne "$1" ]; then
    cat "$fixture_root/output.log" >&2
    printf 'expected Make status %s, got %s\n' "$1" "$actual_status" >&2
    exit 1
  fi
}
expect_installs() {
  local actual
  actual="$(cat "$TEST_INSTALL_LOG")"
  if [ "$actual" != "$1" ]; then
    printf 'unexpected Go installs\nexpected:\n%s\nactual:\n%s\n' "$1" "$actual" >&2
    exit 1
  fi
}

reset_fixture
printf 'go1.27.3\n' > "$fixture_bin/staticcheck.goversion"
printf 'go1.26.1\n' > "$fixture_bin/ineffassign.goversion"
printf 'go1.26.1\n' > "$fixture_bin/deadcode.goversion"
run_make 0
expected="$(printf '%s\n' "${modules[1]}" "${modules[3]}")"
expect_installs "$expected"
run_make 0
expect_installs "$expected"

reset_fixture
for tool in "${tools[@]}"; do rm "$fixture_bin/$tool" "$fixture_bin/$tool.goversion"; done
run_make 0
expect_installs "$(printf '%s\n' "${modules[@]}")"
export TEST_ACTIVE_VERSION=go1.28.0
run_make 0
expect_installs "$(printf '%s\n' "${modules[@]}" "${modules[@]}")"
export TEST_ACTIVE_VERSION=go1.27.1
run_make 0
expect_installs "$(printf '%s\n' "${modules[@]}" "${modules[@]}")"

reset_fixture
export TEST_ACTIVE_VERSION=unknown
run_make 2
grep -F 'cannot read active Go language version' "$fixture_root/output.log" >/dev/null
expect_installs ''
export TEST_ACTIVE_VERSION=go1.27.1

reset_fixture
printf 'unknown\n' > "$fixture_bin/staticcheck.goversion"
run_make 2
grep -F 'cannot read Go build metadata for staticcheck' "$fixture_root/output.log" >/dev/null
expect_installs ''

reset_fixture
export TEST_ACTIVE_FAILURE=1
run_make 2
grep -F 'read active Go toolchain' "$fixture_root/output.log" >/dev/null
unset TEST_ACTIVE_FAILURE
expect_installs ''

reset_fixture
export TEST_METADATA_FAILURE=1
run_make 2
grep -F 'read Go build metadata for staticcheck' "$fixture_root/output.log" >/dev/null
unset TEST_METADATA_FAILURE
expect_installs ''

reset_fixture
rm "$fixture_bin/errcheck" "$fixture_bin/errcheck.goversion" "$fixture_bin/deadcode" "$fixture_bin/deadcode.goversion"
export TEST_INSTALL_FAILURE=errcheck
run_make 2
grep -F 'install errcheck failed with status 42' "$fixture_root/output.log" >/dev/null
unset TEST_INSTALL_FAILURE
expect_installs "${modules[2]}"

reset_fixture
printf 'go1.26.1\n' > "$fixture_bin/staticcheck.goversion"
export TEST_INSTALL_INEFFECTIVE=1
run_make 2
grep -F 'installed staticcheck remains older than the active Go language version' "$fixture_root/output.log" >/dev/null
unset TEST_INSTALL_INEFFECTIVE
expect_installs "${modules[0]}"

reset_fixture
rm "$fixture_bin/staticcheck" "$fixture_bin/staticcheck.goversion"
export TEST_INSTALL_INEFFECTIVE=1
run_make 2
grep -F 'installed staticcheck is unavailable on PATH' "$fixture_root/output.log" >/dev/null
unset TEST_INSTALL_INEFFECTIVE
expect_installs "${modules[0]}"

printf 'Go analyzer Make bootstrap integration passed.\n'
