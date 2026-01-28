#!/bin/bash
# Railway Database Migration Script - Stage Environment
#
# Prerequisites:
# 1. Railway CLI installed: npm install -g @railway/cli
# 2. Logged into Railway: railway login
# 3. Project linked: railway link (select stage environment)
# 4. SSH access to Coolify server (mattcom alias configured)

set -e

BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/stage-backup-${TIMESTAMP}.sql"

echo "=== Railway Stage Database Migration ==="
echo ""

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

echo "Step 1: Exporting database from Coolify..."
echo "  Running pg_dump on ph_stage_db container..."

ssh mattcom "docker exec z88gow84ogw00oc4c4c8808w pg_dump -U ph_stage_user psychic_homily_stage" > "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo "  ✓ Database exported to $BACKUP_FILE"
    echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
else
    echo "  ✗ Failed to export database"
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
echo "=== Stage Database Migration Complete ==="
echo ""
echo "Next steps:"
echo "1. Set environment variables in Railway dashboard"
echo "2. Configure custom domain: stage.api.psychichomily.com"
echo "3. Update DNS CNAME record at name.com"
echo "4. Test the deployment: curl https://stage.api.psychichomily.com/health"
