#!/bin/bash
# Railway Database Migration Script - Production Environment
#
# Prerequisites:
# 1. Railway CLI installed: npm install -g @railway/cli
# 2. Logged into Railway: railway login
# 3. Project linked: railway link (select production environment)
# 4. SSH access to Coolify server (mattcom alias configured)

set -e

BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/prod-backup-${TIMESTAMP}.sql"

echo "=== Railway Production Database Migration ==="
echo ""
echo "⚠️  WARNING: This script migrates PRODUCTION data!"
echo ""
read -p "Are you sure you want to continue? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

echo ""
echo "Step 1: Exporting database from Coolify..."
echo "  Running pg_dump on production container..."

# Note: Update container name if different
ssh mattcom "docker exec nkg0wo808kcosog4004ow8cc pg_dump -U prodpsychicadmin prodpsychicdb" > "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo "  ✓ Database exported to $BACKUP_FILE"
    echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
else
    echo "  ✗ Failed to export database"
    echo "  Try listing containers with: ssh mattcom 'docker ps | grep postgres'"
    exit 1
fi

echo ""
echo "Step 2: Getting Railway DATABASE_PUBLIC_URL..."
RAILWAY_DB_URL=$(railway variables --json | python3 -c "import sys,json; print(json.load(sys.stdin).get('DATABASE_PUBLIC_URL',''))")

if [ -z "$RAILWAY_DB_URL" ]; then
    echo "  ✗ Could not get DATABASE_PUBLIC_URL from Railway"
    echo "  Make sure you're linked to the PostgreSQL service: railway link"
    exit 1
fi
echo "  ✓ Got DATABASE_PUBLIC_URL from Railway"

echo ""
echo "Step 3: Importing database to Railway..."
read -p "Press Enter to continue with import to Railway (Ctrl+C to cancel)..."

psql "$RAILWAY_DB_URL" < "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo "  ✓ Database imported successfully"
else
    echo "  ✗ Failed to import database"
    exit 1
fi

echo ""
echo "Step 4: Verifying tables..."
psql "$RAILWAY_DB_URL" -c "\dt"

echo ""
echo "=== Production Database Migration Complete ==="
echo ""
echo "Next steps:"
echo "1. Set environment variables in Railway dashboard"
echo "2. Configure custom domain: api.psychichomily.com"
echo "3. Update DNS CNAME record at name.com"
echo "4. Test the deployment: curl https://api.psychichomily.com/health"
echo "5. Test OAuth login flow"
echo "6. Keep Coolify running until Railway is fully verified"
