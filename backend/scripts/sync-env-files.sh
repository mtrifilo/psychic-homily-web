#!/bin/bash

# Sync environment files to VPS
# Usage: ./backend/scripts/sync-env-files.sh [stage|production|both]

set -e

# Configuration
VPS_HOST="mattcom"
VPS_USER="deploy"

# Determine script location and set paths accordingly
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

STAGE_ENV_FILE="$PROJECT_ROOT/backend/.env.stage"
PROD_ENV_FILE="$PROJECT_ROOT/backend/.env.production"
STAGE_REMOTE_DIR="/opt/psychic-homily-stage/backend"
PROD_REMOTE_DIR="/opt/psychic-homily-production/backend"

# Systemd service files
STAGE_SERVICE_FILE="$PROJECT_ROOT/backend/systemd/psychic-homily-stage.service"
PROD_SERVICE_FILE="$PROJECT_ROOT/backend/systemd/psychic-homily-production.service"
STAGE_SERVICE_REMOTE="/etc/systemd/system/psychic-homily-stage.service"
PROD_SERVICE_REMOTE="/etc/systemd/system/psychic-homily-production.service"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}🔧${NC} $1"
}

print_success() {
    echo -e "${GREEN}✅${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠️${NC} $1"
}

print_error() {
    echo -e "${RED}❌${NC} $1"
}

# Function to sync a single environment file
sync_env_file() {
    local env_file=$1
    local remote_dir=$2
    local env_name=$3
    
    if [ ! -f "$env_file" ]; then
        print_error "Local environment file not found: $env_file"
        return 1
    fi
    
    print_status "Syncing $env_name environment file..."
    echo "  📁 Local file: $env_file"
    echo "  📁 Remote dir: $remote_dir"
    
    # Create remote directory if it doesn't exist
    ssh "$VPS_USER@$VPS_HOST" "mkdir -p $remote_dir" 2>/dev/null || true
    
    # Copy the file
    if scp "$env_file" "$VPS_USER@$VPS_HOST:$remote_dir/"; then
        print_success "$env_name environment file synced successfully"
        
        # Verify the file was copied
        if ssh "$VPS_USER@$VPS_HOST" "test -f $remote_dir/$(basename $env_file)"; then
            print_success "File verified on VPS: $remote_dir/$(basename $env_file)"
            
            # Additional debugging: show file details on VPS
            echo "  📋 File details on VPS:"
            ssh "$VPS_USER@$VPS_HOST" "ls -la $remote_dir/$(basename $env_file) 2>/dev/null || echo 'File not found'"
        else
            print_warning "File copy verification failed"
            echo "  🔍 Debug: Checking what's in remote directory:"
            ssh "$VPS_USER@$VPS_HOST" "ls -la $remote_dir/ 2>/dev/null || echo 'Directory not accessible'"
        fi
    else
        print_error "Failed to sync $env_name environment file"
        return 1
    fi
}

# Function to sync a single systemd service file
sync_service_file() {
    local service_file=$1
    local remote_path=$2
    local service_name=$3
    
    if [ ! -f "$service_file" ]; then
        print_error "Local service file not found: $service_file"
        return 1
    fi
    
    print_status "Syncing $service_name systemd service file..."
    echo "  📁 Local file: $service_file"
    echo "  📁 Remote path: $remote_path"
    
    # Copy the service file using scp, then move to systemd directory
    if scp "$service_file" "$VPS_USER@$VPS_HOST:/tmp/$(basename $service_file)"; then
        # Copy the file to the systemd directory with sudo
        if ssh "$VPS_USER@$VPS_HOST" "sudo /bin/cp /tmp/$(basename $service_file) $remote_path"; then
        print_success "$service_name service file synced successfully"
        
        # Verify the file was copied
        if ssh "$VPS_USER@$VPS_HOST" "sudo /bin/cp $remote_path /tmp/verify_test >/dev/null 2>&1"; then
            print_success "Service file verified on VPS: $remote_path"
            
            # Reload systemd daemon
            print_status "Reloading systemd daemon..."
            if ssh "$VPS_USER@$VPS_HOST" "sudo systemctl daemon-reload"; then
                print_success "Systemd daemon reloaded successfully"
            else
                print_warning "Failed to reload systemd daemon"
            fi
        else
            print_warning "Service file copy verification failed"
        fi
    else
        print_error "Failed to move service file to systemd directory"
        return 1
    fi
else
    print_error "Failed to copy service file to VPS"
    return 1
fi
}

# Function to sync stage environment
sync_stage() {
    print_status "Syncing STAGE environment..."
    sync_env_file "$STAGE_ENV_FILE" "$STAGE_REMOTE_DIR" "stage"
    sync_service_file "$STAGE_SERVICE_FILE" "$STAGE_SERVICE_REMOTE" "stage"
}

# Function to sync production environment
sync_production() {
    print_status "Syncing PRODUCTION environment..."
    sync_env_file "$PROD_ENV_FILE" "$PROD_REMOTE_DIR" "production"
    sync_service_file "$PROD_SERVICE_FILE" "$PROD_SERVICE_REMOTE" "production"
}

# Function to sync both environments
sync_both() {
    print_status "Syncing BOTH environments..."
    sync_stage
    sync_production
}

# Main script logic
main() {
    echo "🚀 Environment File Sync Script"
    echo "================================"
    
    # Check if environment files exist
    if [ ! -f "$STAGE_ENV_FILE" ] || [ ! -f "$PROD_ENV_FILE" ]; then
        print_error "Environment files not found"
        echo "Expected files:"
        echo "  - $STAGE_ENV_FILE"
        echo "  - $PROD_ENV_FILE"
        echo ""
        echo "Please ensure you have .env.stage and .env.production files in your backend/ directory"
        exit 1
    fi
    
    # Parse command line argument
    case "${1:-both}" in
        "stage"|"staging")
            sync_stage
            ;;
        "production"|"prod")
            sync_production
            ;;
        "both"|"all")
            sync_both
            ;;
        *)
            print_error "Invalid argument: $1"
            echo "Usage: $0 [stage|production|both]"
            echo "Default: both (if no argument provided)"
            exit 1
            ;;
    esac
    
    echo ""
    print_success "Environment and service sync completed!"
    echo ""
    echo "📋 Summary:"
    echo "  - Stage: $STAGE_REMOTE_DIR/.env.stage + systemd service"
    echo "  - Production: $PROD_REMOTE_DIR/.env.production + systemd service"
    echo ""
    echo "💡 Tip: You can now run this script whenever you update environment files or systemd services:"
    echo "  ./backend/scripts/sync-env-files.sh stage      # Sync only stage"
    echo "  ./backend/scripts/sync-env-files.sh production # Sync only production"
    echo "  ./backend/scripts/sync-env-files.sh both       # Sync both (default)"
}

# Run the main function with all arguments
main "$@"
