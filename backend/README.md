# Psychic Homily Backend API

A Go-based REST API for the Psychic Homily music website, built with Huma framework and PostgreSQL.

## Features

- RESTful API with OpenAPI documentation
- PostgreSQL database with GORM ORM
- Docker containerization
- Database migrations with golang-migrate
- Graceful shutdown handling
- CORS support for frontend integration
- Automated backups to Google Cloud Storage
- Production deployment scripts

## Tech Stack

- **Framework**: Huma v2 (Go)
- **Database**: PostgreSQL 17.5
- **ORM**: GORM
- **Migrations**: golang-migrate
- **Containerization**: Docker & Docker Compose
- **Router**: Chi
- **Backup Storage**: Google Cloud Storage
- **Production**: Nginx, Let's Encrypt SSL

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.24+ (for local development)

### Using Docker (Recommended)

1. **Start all services (including migrations):**

   ```bash
   docker compose up -d

   // run everything except the app
   docker compose up db migrate pgadmin -d
   ```

2. **Check if services are running:**

   ```bash
   docker compose ps
   ```

3. **View logs:**

   ```bash
   docker compose logs -f
   ```

4. **Test the API:**
   ```bash
   curl http://localhost:8080/health
   ```

### Local Development

#### Option 1: Database in Docker, App Locally (Recommended)

This approach provides the fastest development cycle with hot reloading and better debugging.

1. **Start only the database and run migrations:**

   ```bash
   docker compose up -d db migrate
   ```

2. **Run the API locally:**

   ```bash
   go run cmd/server/main.go
   ```

3. **Test the API:**
   ```bash
   curl http://localhost:8080/health
   ```

#### Option 2: Everything in Docker

Use this for testing the full containerized environment.

```bash
docker compose up -d
```

#### Hot Reload Development (Optional)

For automatic restarts when you make code changes:

1. **Install air for hot reload:**

   ```bash
   go install github.com/cosmtrek/air@latest
   ```

2. **Run with hot reload:**
   ```bash
   air
   ```

#### Development Workflow

1. **Make code changes** - Edit your Go files
2. **Auto-restart** - If using `air`, the app restarts automatically
3. **Manual restart** - If using `go run`, restart the process (Ctrl+C, then `go run cmd/server/main.go`)
4. **Test changes** - `curl http://localhost:8080/health`

#### Environment Configuration

The app automatically loads the correct environment file:

- **Development**: `.env.development` (loaded when `NODE_ENV=development`)
- **Production**: `.env.production` (loaded when `NODE_ENV=production`)

The database connection is configured for Docker networking (`db:5432`) when running in containers.

## Deployment commands to run

### Development

docker-compose up -d

### Production

docker-compose -f docker-compose.prod.yml up -d

### Production with explicit env

NODE_ENV=production docker-compose -f docker-compose.prod.yml up -d

## Management Scripts

The project includes several scripts for common operations. All scripts are located in the `backend/scripts/` directory.

### **Backup and Restore Scripts**

#### **backup-to-gcs.sh**

Creates a database backup and uploads it to Google Cloud Storage.

```bash
# Create backup and upload to GCS
./scripts/backup-to-gcs.sh

# Output:
# Backup created: backups/backup_20250119_020000.sql
# Backup uploaded to: gs://psychic-homily-backups/backups/backup_20250119_020000.sql
# Monthly cost: ~$0.12 (for daily backups)
```

**Features:**

- Creates PostgreSQL dump
- Uploads to Google Cloud Storage
- Keeps 7 days of local backups
- Keeps 30 days of remote backups
- Automatic cleanup of old backups

#### **restore-from-gcs.sh**

Restores database from a backup stored in Google Cloud Storage.

```bash
# Restore from specific backup
./scripts/restore-from-gcs.sh backup_20250119_020000.sql

# List available backups
./scripts/restore-from-gcs.sh

# Restore from GCS URL
./scripts/restore-from-gcs.sh gs://psychic-homily-backups/backups/backup_20250119_020000.sql
```

**Features:**

- Downloads backup from GCS if not local
- Supports both local and remote backup files
- Validates backup file existence
- Restores to production database

#### **verify-gcs-backups.sh**

Verifies the integrity of backups stored in Google Cloud Storage.

```bash
# Verify backup integrity
./scripts/verify-gcs-backups.sh

# Output:
# === Psychic Homily Backup Verification Report ===
# Date: Sat Jul 19 21:30:00 UTC 2025
#
# Local backups: 7
# Remote backups in GCS: 30
# Latest backup: gs://psychic-homily-backups/backups/backup_20250119_020000.sql
# ✅ Latest backup is accessible
#
# Monthly cost estimate: ~$0.12
```

**Features:**

- Counts local and remote backups
- Tests download accessibility
- Shows latest backup information
- Estimates monthly costs

### **Deployment Scripts**

#### **deploy-to-production.sh**

Deploys the application to production environment.

```bash
# Deploy to production
./scripts/deploy-to-production.sh

# Output:
# 🚀 Deploying Psychic Homily Backend...
# Generating strong database password...
# Waiting for database to be ready...
# ✅ Deployment complete!
# 🌐 API available at: https://psychichomily.com/api/
# 💾 Database backup created
```

**Features:**

- Generates strong passwords if not set
- Stops existing services
- Builds and starts new containers
- Runs database migrations
- Creates initial backup
- Waits for database health

#### **update-production.sh**

Updates the application in production with latest code.

```bash
# Update production
./scripts/update-production.sh

# Output:
# 🔄 Updating Psychic Homily Backend...
# ✅ Update complete!
```

**Features:**

- Pulls latest code from Git
- Rebuilds containers
- Runs migrations
- Minimal downtime deployment

### **Automated Backup Scheduling**

Set up automated daily backups:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * /path/to/your/project/backend/scripts/backup-to-gcs.sh >> /var/log/backup.log 2>&1

# Check backup logs
tail -f /var/log/backup.log
```

### **Script Usage Examples**

#### **Complete Backup Workflow**

```bash
# 1. Create backup
./scripts/backup-to-gcs.sh

# 2. Verify backup integrity
./scripts/verify-gcs-backups.sh

# 3. List available backups
./scripts/restore-from-gcs.sh

# 4. Restore if needed
./scripts/restore-from-gcs.sh backup_20250119_020000.sql
```

#### **Production Deployment Workflow**

```bash
# 1. Deploy to production
./scripts/deploy-to-production.sh

# 2. Update with new code
./scripts/update-production.sh

# 3. Verify deployment
curl https://psychichomily.com/api/health
```

#### **Emergency Recovery**

```bash
# 1. Check available backups
./scripts/restore-from-gcs.sh

# 2. Restore from latest backup
./scripts/restore-from-gcs.sh $(gsutil ls gs://psychic-homily-backups/backups/ | tail -1)

# 3. Verify restoration
docker compose -f docker-compose.prod.yml exec db psql -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT COUNT(*) FROM shows;"
```

### **Script Configuration**

All scripts use environment variables from `.env.production`:

```bash
# Required environment variables
DATABASE_URL=postgres://psychicadmin:${PROD_DB_PASSWORD}@localhost:5432/psychicdb_prod
POSTGRES_USER=psychicadmin
POSTGRES_PASSWORD=${PROD_DB_PASSWORD}
POSTGRES_DB=psychicdb_prod
GCS_BUCKET=psychic-homily-backups
```

### **Troubleshooting Scripts**

#### **Common Issues**

**Backup fails:**

```bash
# Check GCS authentication
gcloud auth application-default login

# Verify bucket exists
gsutil ls gs://psychic-homily-backups/

# Check environment variables
cat .env.production
```

**Restore fails:**

```bash
# Check backup file exists
gsutil ls gs://psychic-homily-backups/backups/ | grep backup_20250119_020000.sql

# Verify database connection
docker compose -f docker-compose.prod.yml exec db pg_isready -U $POSTGRES_USER
```

**Deployment fails:**

```bash
# Check Docker Compose file
docker compose -f docker-compose.prod.yml config

# View deployment logs
docker compose -f docker-compose.prod.yml logs api
```

## Database Migrations

### Migration Workflow

The project uses `golang-migrate` for database schema management, automatically run via Docker Compose.

#### **First Time Setup**

```bash
# Start database and run all migrations
docker compose up -d db migrate

# Verify migrations completed
docker compose logs migrate
```

#### **Creating New Migrations**

```bash
# Create new migration files
migrate create -ext sql -dir db/migrations -seq add_user_table

# This creates:
# db/migrations/000002_add_user_table.up.sql
# db/migrations/000002_add_user_table.down.sql
```

#### **Running Migrations**

```bash
# Run all pending migrations
docker compose run --rm migrate

# Run migrations and see output
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" up

# Check migration status
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" version

# Check migration history
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" version
```

#### **Rolling Back Migrations**

```bash
# Rollback one migration
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" down 1

# Rollback to specific version
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" goto 1

# Rollback all migrations
docker compose run --rm migrate -path /migrations -database "postgres://psychicadmin:secretpassword@db:5432/psychicdb?sslmode=disable" down
```

#### **Development Reset**

```bash
# Complete reset (removes all data and runs migrations)
docker compose down -v
docker compose up -d

# Reset database only (keeps volumes)
docker compose down
docker compose up -d db migrate
```

### Migration Best Practices

#### **File Naming Convention**

```markdown
db/migrations/
├── 000001_create_initial_schema.up.sql
├── 000001_create_initial_schema.down.sql
├── 000002_add_user_table.up.sql
├── 000002_add_user_table.down.sql
└── ...
```

#### **Migration Guidelines**

- **Always create both `.up.sql` and `.down.sql` files**
- **Test rollbacks** before committing migrations
- **Use descriptive names** that explain the change
- **Keep migrations small and focused** - one logical change per migration
- **Never modify existing migrations** - create new ones instead
- **Use transactions** for complex migrations

## Essential Docker Compose Commands

### Basic Operations

```bash
# Start all services in background
docker compose up -d

# Start and see logs
docker compose up

# Stop all services
docker compose down

# Stop and remove volumes (WARNING: deletes database data)
docker compose down -v

# Build images
docker compose build

# Build and start
docker compose up -d --build

# Force rebuild (ignores cache)
docker compose build --no-cache
```

### Service Management

```bash
# Start specific service
docker compose up -d api
docker compose up -d db

# Stop specific service
docker compose stop api

# Restart specific service
docker compose restart api

# Check running services
docker compose ps
```

### Logs and Monitoring

```bash
# All services logs
docker compose logs

# Follow logs in real-time
docker compose logs -f

# Specific service logs
docker compose logs api
docker compose logs db

# Last N lines
docker compose logs --tail=50
```

### Database Operations

```bash
# Connect to Postgres directly
docker compose exec db psql -U psychicadmin -d psychicdb

# Create database backup
docker compose exec db pg_dump -U psychicadmin psychicdb > backup.sql

# Run SQL file
docker compose exec -T db psql -U psychicadmin -d psychicdb < backup.sql

# Check database health
docker compose exec db pg_isready -U psychicadmin
```

### Development Workflows

```bash
# Fresh start (removes all data)
docker compose down -v
docker compose up -d --build

# Quick restart (API changes)
docker compose restart api

# Reset database only
docker compose down
docker compose up -d db

# Access container shells
docker compose exec api sh
docker compose exec db bash
```

## API Endpoints

### Health Check

```bash
GET /health
```

**Response:**

```json
{
  "body": {
    "status": "ok"
  }
}
```

### Submit Show

```bash
POST /show
Content-Type: application/json
```

**Request Body:**

```json
{
  "artists": [
    {
      "name": "Psychic Homily",
      "instagram": "psychichomily",
      "bandcamp": "psychichomily.bandcamp.com"
    }
  ],
  "venue": "Valley Bar",
  "date": "2025-01-15",
  "cost": "15",
  "ages": "21+",
  "city": "Phoenix",
  "state": "AZ"
}
```

**Response:**

```json
{
  "body": {
    "success": true,
    "show_id": "1234567890"
  }
}
```

## Environment Variables

### Environment Files

The project uses environment-specific configuration files:

- `.env.example` - Template with all available variables
- `.env.development` - Development environment settings
- `.env.production` - Production environment settings

### Setting Environment

```bash
# Development (default)
NODE_ENV=development docker compose up

# Production
NODE_ENV=production docker compose up
```

### Environment Variables

| Variable               | Default                     | Description                              |
| ---------------------- | --------------------------- | ---------------------------------------- |
| `NODE_ENV`             | `development`               | Environment name                         |
| `API_ADDR`             | `127.0.0.1:8080`            | API server address                       |
| `API_PORT`             | `8080`                      | API server port                          |
| `DATABASE_URL`         | `postgres://...`            | Database connection string               |
| `POSTGRES_USER`        | `psychicadmin`              | Database username                        |
| `POSTGRES_PASSWORD`    | `secretpassword`            | Database password                        |
| `POSTGRES_DB`          | `psychicdb`                 | Database name                            |
| `POSTGRES_HOST`        | `db`                        | Database host                            |
| `POSTGRES_PORT`        | `5432`                      | Database port                            |
| `CORS_ALLOWED_ORIGINS` | `https://psychichomily.com` | Comma-separated CORS origins             |
| `LOG_LEVEL`            | `debug`                     | Logging level (debug, info, warn, error) |

### Security Notes

- **Never commit `.env.production`** to version control
- **Use strong passwords** in production
- **Rotate credentials** regularly
- **Use secrets management** in production (Docker Secrets, Kubernetes Secrets, etc.)

## Database Schema

The API uses PostgreSQL with the following main tables:

- **artists** - Artist/band information
- **venues** - Venue information
- **shows** - Concert/show information
- **show_artists** - Many-to-many relationship between shows and artists

### Schema Migration History

- `000001_create_initial_schema` - Initial tables (artists, venues, shows, show_artists)

## Development

## Venue Discovery

The project includes an automated venue discovery system that imports show data from venue calendars.

### Components

- **Node.js Discovery** (`discovery/`) - Playwright-based discovery tool for TicketWeb venues
- **Go Importer** (`cmd/discovery-import/`) - CLI tool to import discovered JSON into the database
- **Systemd Timer** (`deploy/discovery/`) - Weekly scheduled runs on the server

### Usage

```bash
# Run discovery and import (from project root)
cd discovery
./run-discovery.sh

# Dry run (no database changes)
./run-discovery.sh --dry-run

# Import only (if you have JSON files)
cd backend
go build -o ./discovery-import ./cmd/discovery-import
./discovery-import -input ../discovery/output/discovered-events-*.json -dry-run
```

### Server Deployment

```bash
# Install systemd timer
sudo cp deploy/discovery/discovery.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now discovery.timer

# Manual run
sudo systemctl start discovery.service
journalctl -u discovery.service -f
```

See `docs/venue-discovery-design.md` for detailed documentation.

---

### Project Structure

```
backend/
├── cmd/
│   ├── server/
│   │   └── main.go              # Application entry point
│   └── discovery-import/
│       └── main.go              # Venue discovery importer CLI
├── internal/
│   ├── api/
│   │   ├── handlers/            # HTTP handlers
│   │   │   └── health.go        # Health endpoint
│   │   └── routes/              # Route definitions
│   │       └── routes.go        # All routes setup
│   └── config/                  # Configuration management
│       └── config.go            # Environment variable handling
├── db/
│   └── migrations/              # Database migration files
│       ├── 000001_create_initial_schema.up.sql
│       └── 000001_create_initial_schema.down.sql
├── scripts/                     # Management scripts
│   ├── backup-to-gcs.sh         # Backup to Google Cloud Storage
│   ├── restore-from-gcs.sh      # Restore from GCS backup
│   ├── deploy-to-production.sh  # Deploy to production
│   ├── update-production.sh     # Update production
│   └── verify-gcs-backups.sh    # Verify backup integrity
├── docs/                        # Documentation
│   └── venue-discovery-design.md # Venue discovery architecture
├── deploy/                      # Deployment configurations
│   └── discovery/               # Systemd units for venue discovery
├── Dockerfile                   # Docker image definition
├── docker-compose.yml           # Docker Compose configuration
├── docker-compose.prod.yml      # Production Docker Compose
└── README.md                    # This file
```
