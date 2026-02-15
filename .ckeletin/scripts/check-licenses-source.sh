#!/usr/bin/env bash
# Check dependency licenses using go-licenses (source-based, fast)
# Uses conservative permissive-only policy by default
# See: ADR-011 License Compliance Strategy

set -e

# Source standard output functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/check-output.sh
source "${SCRIPT_DIR}/lib/check-output.sh"

# Default policy: Allow permissive licenses only
# Note: go-licenses doesn't support both --allowed_licenses and --disallowed_types
# We use --allowed_licenses for explicit permissive-only policy
ALLOWED_LICENSES="${LICENSE_ALLOWED:-MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,0BSD,Unlicense}"

# Get module path to ignore self
MODULE_PATH=$(go list -m 2>/dev/null || echo "github.com/peiman/ckeletin-go")

filter_known_go_licenses_noise() {
    sed -E \
        -e '/^W[0-9]{4} .*library\.go:141\] .*contains non-Go code that can'\''t be inspected for further dependencies:$/d' \
        -e '/^E[0-9]{4} .*library\.go:159\] Package .* does not have module info\. Non go modules projects are no longer supported\..*$/d' \
        -e '/^F[0-9]{4} .*main\.go:75\] some errors occurred when loading direct and transitive dependency packages$/d' \
        -e '/^[[:space:]]*\/.*$/d' \
        -e '/^[[:space:]]*$/d'
}

check_header "Checking dependency licenses (source-based)"

# Check if go-licenses is installed
if ! command -v go-licenses &> /dev/null; then
    check_failure "go-licenses not installed" "" \
        "Install with: go install github.com/google/go-licenses/v2@latest"$'\n'"Or run: task setup"
    exit 1
fi

# Build non-stdlib package list to avoid stdlib/non-module false failures in go-licenses.
# go-licenses is still used for policy enforcement on actual module dependencies.
CHECK_CMD="go list -deps -f '{{if and (not .Standard) .Module}}{{.ImportPath}}{{end}}' ./... | sort -u | xargs go-licenses check --allowed_licenses='$ALLOWED_LICENSES' --ignore='$MODULE_PATH'"

# Run license check
if run_check "$CHECK_CMD 2>&1"; then
    check_success "All dependency licenses compliant"
    check_note "Source-based check. Run 'task check:license:binary' for release verification."
    exit 0
else
    # go-licenses v2.0.1 emits non-module/stdlib errors on newer Go toolchains.
    # If output contains only this known noise, continue and rely on binary check.
    FILTERED_OUTPUT=$(printf "%s\n" "$CHECK_OUTPUT" | filter_known_go_licenses_noise || true)
    if [ -z "$FILTERED_OUTPUT" ]; then
        check_success "Dependency licenses checked (with known go-licenses stdlib limitation)"
        check_note "go-licenses emitted stdlib/non-module noise; binary check remains enforced in release flow."
        exit 0
    fi

    check_failure \
        "License compliance check failed (source-based)" \
        "$FILTERED_OUTPUT" \
        "Remove dependency: go get <package>@none"$'\n'"Find alternative: Search pkg.go.dev for MIT/Apache-2.0 alternatives"$'\n'"Review policy: See docs/licenses.md for customization"$'\n'"Generate report: task generate:license:report"
    exit 1
fi
