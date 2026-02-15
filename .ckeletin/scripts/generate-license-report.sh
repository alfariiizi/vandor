#!/usr/bin/env bash
# Generate license report (CSV format) using go-licenses
# Used for audits, documentation, and NOTICE generation
# See: ADR-011 License Compliance Strategy

set -e

OUTPUT_DIR="${LICENSE_REPORT_DIR:-reports}"
OUTPUT_FILE="$OUTPUT_DIR/licenses.csv"
ERROR_LOG="$OUTPUT_DIR/license-errors.log"
FALLBACK_TOOLCHAIN="${LICENSE_REPORT_FALLBACK_TOOLCHAIN:-go1.24.1+auto}"

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

echo "📊 Generating license report..."
echo "   Tool: go-licenses"
echo "   Output: $OUTPUT_FILE"
echo ""

# Check if go-licenses is installed
if ! command -v go-licenses &> /dev/null; then
    echo "❌ go-licenses not installed"
    echo ""
    echo "Install with:"
    echo "  go install github.com/google/go-licenses/v2@latest"
    echo ""
    echo "Or run:"
    echo "  task setup"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Generate report
echo "Scanning dependencies..."
PKG_LIST=$(go list -deps -f '{{if and (not .Standard) .Module}}{{.ImportPath}}{{end}}' ./... | sort -u)
if [ -z "$PKG_LIST" ]; then
    echo "❌ Failed to resolve dependency package list"
    exit 1
fi

if echo "$PKG_LIST" | xargs go-licenses report \
    --ignore="$MODULE_PATH" > "$OUTPUT_FILE" 2> "$ERROR_LOG"; then

    # Count dependencies
    DEP_COUNT=$(($(wc -l < "$OUTPUT_FILE") - 1))  # Subtract header line

    echo ""
    echo "✅ License report generated successfully"
    echo "   File: $OUTPUT_FILE"
    echo "   Dependencies: $DEP_COUNT"

    # Show sample (first 10 lines)
    if [ "$DEP_COUNT" -gt 0 ]; then
        echo ""
        echo "Sample (first 5 dependencies):"
        echo "----------------------------------------"
        head -n 6 "$OUTPUT_FILE" | column -t -s ','
        if [ "$DEP_COUNT" -gt 5 ]; then
            echo "... and $((DEP_COUNT - 5)) more"
        fi
        echo "----------------------------------------"
    fi

    # Check for errors
    if [ -s "$ERROR_LOG" ]; then
        echo ""
        echo "⚠️  Warnings/errors during scan (see $ERROR_LOG):"
        head -n 5 "$ERROR_LOG"
    else
        rm -f "$ERROR_LOG"
    fi

    echo ""
    echo "Use this report for:"
    echo "  - Compliance audits"
    echo "  - NOTICE file generation (task generate:attribution)"
    echo "  - Dependency documentation"

    exit 0
fi

echo ""
echo "⚠️  Primary license report generation failed, checking if this is known go-licenses stdlib noise..."
FILTERED_ERRORS=$(cat "$ERROR_LOG" | filter_known_go_licenses_noise || true)
if [ -n "$FILTERED_ERRORS" ]; then
    echo "❌ Failed to generate license report"
    echo ""
    echo "Errors:"
    echo "$FILTERED_ERRORS"
    exit 1
fi

echo "Known go-licenses stdlib/non-module issue detected."
echo "Retrying with fallback toolchain: $FALLBACK_TOOLCHAIN"
if echo "$PKG_LIST" | xargs env GOTOOLCHAIN="$FALLBACK_TOOLCHAIN" go-licenses report \
    --ignore="$MODULE_PATH" > "$OUTPUT_FILE" 2> "$ERROR_LOG"; then
    DEP_COUNT=$(wc -l < "$OUTPUT_FILE")
    echo ""
    echo "✅ License report generated successfully via fallback toolchain"
    echo "   File: $OUTPUT_FILE"
    echo "   Dependencies: $DEP_COUNT"
    if [ -s "$ERROR_LOG" ]; then
        echo ""
        echo "⚠️  Warnings during scan (see $ERROR_LOG):"
        head -n 5 "$ERROR_LOG"
    fi
    exit 0
fi

FILTERED_FALLBACK_ERRORS=$(cat "$ERROR_LOG" | filter_known_go_licenses_noise || true)
if [ -z "$FILTERED_FALLBACK_ERRORS" ]; then
    : > "$OUTPUT_FILE"
    echo ""
    echo "⚠️  License report generated as empty due to known go-licenses stdlib/non-module limitation."
    echo "   See details in: $ERROR_LOG"
    exit 0
fi

echo ""
echo "❌ Failed to generate license report (including fallback)"
echo ""
if [ -s "$ERROR_LOG" ]; then
    echo "Errors:"
    echo "$FILTERED_FALLBACK_ERRORS"
fi
exit 1
