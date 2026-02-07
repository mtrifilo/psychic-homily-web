#!/bin/bash
#
# Venue Discovery Wrapper Script
# Runs the Node.js discovery tool and imports results into the database
#
# Usage:
#   ./run-discovery.sh              # Normal run
#   ./run-discovery.sh --dry-run    # Dry run (no database changes)
#

set -e  # Exit on error

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/output"
BACKEND_DIR="${SCRIPT_DIR}/../../backend"
KEEP_DAYS=7  # Keep JSON files for this many days

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse arguments
DRY_RUN=""
VERBOSE=""
for arg in "$@"; do
    case $arg in
        --dry-run)
            DRY_RUN="--dry-run"
            ;;
        --verbose|-v)
            VERBOSE="--verbose"
            ;;
    esac
done

# Check dependencies
if ! command -v bun &> /dev/null; then
    log_error "Bun is not installed. Install from https://bun.sh"
    exit 1
fi

echo ""
echo "========================================"
echo "  Psychic Homily Venue Discovery"
echo "========================================"
echo ""

# Step 1: Run the Node.js discovery tool
log_info "Running venue discovery..."
cd "${SCRIPT_DIR}"

# Ensure dependencies are installed
if [ ! -d "node_modules" ]; then
    log_info "Installing dependencies..."
    bun install
fi

# Run discovery with output flag
bun run scrape-ticketweb-venue.js --all --output "${OUTPUT_DIR}"

if [ $? -ne 0 ]; then
    log_error "Discovery failed"
    exit 1
fi

# Find the most recent JSON file
LATEST_JSON=$(ls -t "${OUTPUT_DIR}"/scraped-events-*.json 2>/dev/null | head -n 1)

if [ -z "${LATEST_JSON}" ]; then
    log_error "No JSON files found in ${OUTPUT_DIR}"
    exit 1
fi

log_info "Latest discovery file: ${LATEST_JSON}"

# Step 2: Build the Go importer (if needed)
log_info "Building Go importer..."
cd "${BACKEND_DIR}"

# Check if binary exists and is up to date
IMPORTER_BIN="${BACKEND_DIR}/discovery-import"
IMPORTER_SRC="${BACKEND_DIR}/cmd/discovery-import/main.go"

if [ ! -f "${IMPORTER_BIN}" ] || [ "${IMPORTER_SRC}" -nt "${IMPORTER_BIN}" ]; then
    go build -o "${IMPORTER_BIN}" ./cmd/discovery-import
    if [ $? -ne 0 ]; then
        log_error "Failed to build importer"
        exit 1
    fi
    log_info "Importer built successfully"
else
    log_info "Using existing importer binary"
fi

# Step 3: Run the importer
log_info "Importing events to database..."
cd "${BACKEND_DIR}"

# Determine env file to use
ENV_FILE=""
if [ -f ".env.development" ]; then
    ENV_FILE=".env.development"
elif [ -f ".env" ]; then
    ENV_FILE=".env"
fi

IMPORT_ARGS="-input ${LATEST_JSON}"
if [ -n "${ENV_FILE}" ]; then
    IMPORT_ARGS="${IMPORT_ARGS} -env ${ENV_FILE}"
fi
if [ -n "${DRY_RUN}" ]; then
    IMPORT_ARGS="${IMPORT_ARGS} ${DRY_RUN}"
fi
if [ -n "${VERBOSE}" ]; then
    IMPORT_ARGS="${IMPORT_ARGS} ${VERBOSE}"
fi

"${IMPORTER_BIN}" ${IMPORT_ARGS}

IMPORT_EXIT_CODE=$?

# Step 4: Clean up old JSON files
log_info "Cleaning up old JSON files (keeping ${KEEP_DAYS} days)..."
find "${OUTPUT_DIR}" -name "scraped-events-*.json" -mtime +${KEEP_DAYS} -delete 2>/dev/null || true

# Summary
echo ""
echo "========================================"
if [ ${IMPORT_EXIT_CODE} -eq 0 ]; then
    log_info "Discovery run completed successfully"
else
    log_warn "Discovery completed with some errors"
fi
echo "========================================"
echo ""

exit ${IMPORT_EXIT_CODE}
