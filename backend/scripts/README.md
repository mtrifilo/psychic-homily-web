# Management Scripts

This directory contains management scripts for the Psychic Homily backend.

## Scripts Overview

### Deployment

- **`deploy.sh`** - Main deployment script (handles both initial deployment and updates)
  - Usage: `./scripts/deploy.sh [--initial|--update]`
  - Automatically creates backups before updates
  - Runs database migrations
  - Verifies application health

### Backup and Restore

- **`backup.sh`** - Creates database backup

  - Usage: `./scripts/backup.sh [--upload] [--verify]`
  - Creates timestamped backups
  - Optional GCS upload with `--upload` flag
  - Optional integrity verification with `--verify` flag
  - Automatically cleans up old backups (local: last 10, GCS: last 30)

- **`restore.sh`** - Restores database from backup
  - Usage: `./scripts/restore.sh <backup_file> [--from-gcs]`
  - Creates pre-restore backup for safety
  - Confirms before overwriting data
  - Supports local and GCS backups
  - Restarts application after restore

### Verification

- **`verify.sh`** - System and backup verification
  - Usage: `./scripts/verify.sh [--backups] [--system] [--all]`
  - Checks system health (Docker, containers, API, database)
  - Verifies backup integrity and accessibility
  - Monitors disk and memory usage

## Quick Start

### Initial Deployment

```bash
# From backend/ directory
cp env.production.example .env.production
# Edit .env.production with your values
./scripts/deploy.sh --initial
```

### Update Deployment

```bash
# From backend/ directory
./scripts/deploy.sh --update
```

### Backup Database

```bash
# Local backup only
./scripts/backup.sh

# Backup with integrity verification
./scripts/backup.sh --verify

# Backup and upload to GCS
./scripts/backup.sh --upload

# Backup with verification and GCS upload
./scripts/backup.sh --upload --verify
```

### Restore Database

```bash
# List available backups
ls -la backups/

# Restore from local backup
./scripts/restore.sh backups/backup_20250101_120000.sql

# Restore from GCS backup
./scripts/restore.sh backup_20250101_120000.sql --from-gcs
```

### Verify System

```bash
# Check everything
./scripts/verify.sh

# Check only system health
./scripts/verify.sh --system

# Check only backups
./scripts/verify.sh --backups
```

## Requirements

- Docker and Docker Compose
- PostgreSQL 17.5 (latest as of 2025)
- Google Cloud SDK (`gsutil`) - optional for GCS backups
- Environment variables in `.env.production`

## Security Features

- All scripts use environment variables for sensitive data
- Automatic pre-deployment/restore backups
- Database credentials loaded from `.env.production`
- GCS bucket name configured via `GCS_BUCKET` environment variable
- Confirmation prompts for destructive operations
- Backup integrity verification

## File Structure

```
backend/
├── scripts/
│   ├── deploy.sh          # Main deployment script
│   ├── backup.sh          # Database backup (local + GCS)
│   ├── restore.sh         # Database restore (local + GCS)
│   ├── verify.sh          # System and backup verification
│   └── README.md          # This file
├── docker-compose.prod.yml # Production Docker setup
├── env.production.example  # Environment template
└── backups/               # Database backups (created automatically)
```

## Automated Backups

Set up automated daily backups with cron:

```bash
# Add to crontab (crontab -e)
0 2 * * * cd /path/to/backend && ./scripts/backup.sh --upload >> /var/log/backup.log 2>&1
```

## Monitoring

Set up system monitoring with cron:

```bash
# Add to crontab (crontab -e)
*/30 * * * * cd /path/to/backend && ./scripts/verify.sh --system >> /var/log/health.log 2>&1
```
