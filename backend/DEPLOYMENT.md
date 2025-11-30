# Psychic Homily Deployment Guide (Staged Zero-Downtime)

This guide explains how to deploy the Psychic Homily application using staged zero-downtime deployment with Go binaries for backend and Docker infrastructure services, plus automated frontend deployment via Netlify.

## 🚀 Staged Deployment Architecture

### Deployment Strategy:

1. **Backend First**: API changes deploy before frontend
2. **Health Validation**: New binary must pass health checks
3. **Stabilization Period**: Backend runs for 60+ seconds before frontend deploys
4. **Frontend Second**: Frontend deploys only after backend is confirmed stable
5. **Rollback Safety**: If either stage fails, previous versions remain active

### What Runs as Binary:

- **Go API Application**: Fast, native performance, zero-downtime deployments

### What Runs in Docker:

- **PostgreSQL Database**: Easy management, backups, scaling
- **Redis Cache**: Future-ready for caching and sessions
- **Migration Service**: Database schema management

### What Deploys via Netlify:

- **Hugo Frontend**: Static site generation with multi-environment support
- **Automated Builds**: Triggered via webhooks from GitHub Actions
- **Environment-Specific Configs**: Stage and production configurations

## 🏗️ VPS Setup Requirements

### 1. Install Required Software

```bash
# Install Docker and Docker Compose
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# Install Go (for building if needed)
sudo apt update
sudo apt install -y golang-go curl
```

### 2. Create Application Directories

```bash
# Stage environment
sudo mkdir -p /opt/psychic-homily-stage
sudo chown $USER:$USER /opt/psychic-homily-stage

# Production environment
sudo mkdir -p /opt/psychic-homily-production
sudo chown $USER:$USER /opt/psychic-homily-production

cd /opt/psychic-homily-stage  # or production
```

### 3. Set Up Systemd Services

```bash
# Stage environment
sudo cp systemd/psychic-homily-stage.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable psychic-homily-stage

# Production environment
sudo cp systemd/psychic-homily-production.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable psychic-homily-production

# Check status
sudo systemctl status psychic-homily-stage
sudo systemctl status psychic-homily-production
```

### 4. Configure Environment Variables

Create environment files for each environment:

**Stage Environment** (`backend/.env.stage`):

```bash
# Database
POSTGRES_USER=stage_user
POSTGRES_PASSWORD=secure_stage_password
POSTGRES_DB=psychic_homily_stage
DATABASE_URL=postgres://stage_user:secure_stage_password@localhost:5433/stage_db?sslmode=disable

# API
API_ADDR=0.0.0.0:8081
ENVIRONMENT=stage
```

**Production Environment** (`backend/.env.production`):

```bash
# Database
POSTGRES_USER=prodpsychicadmin
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=prodpsychicdb
DATABASE_URL=postgres://prodpsychicadmin:your_password@localhost:5432/prodpsychicdb?sslmode=disable

# API
API_ADDR=0.0.0.0:8080
ENVIRONMENT=production
```

**Important**: Use `localhost` (not `db`) in DATABASE_URL since the binary runs on the host, not inside Docker.

## Deployment Process

### Automated Staged Deployment (GitHub Actions)

1. **Push to main branch** with changes
2. **GitHub Actions** automatically:
   - **Stage 1**: Deploys backend if backend files changed
   - **Wait Period**: 60+ seconds for backend stabilization
   - **Stage 2**: Deploys frontend if frontend files changed

### Production Deployment (Manual Trigger)

1. **Go to GitHub Actions** → **Deploy to Production**
2. **Select deployment target**: backend, frontend, or both
3. **Type "PRODUCTION"** to confirm (safety measure)
4. **Workflow runs** with production-specific configurations

### Manual Staged Deployment

```bash
# Deploy only backend
# Go to GitHub Actions → Deploy Staged → Run workflow → Select 'backend'

# Deploy only frontend
# Go to GitHub Actions → Deploy Staged → Run workflow → Select 'frontend'

# Deploy both (backend first, then frontend)
# Go to GitHub Actions → Deploy Staged → Run workflow → Select 'both'
```

### Manual VPS Deployment

```bash
# Stage environment
cd /opt/psychic-homily-stage
./backend/scripts/deploy-stage.sh <commit-sha>

# Production environment
cd /opt/psychic-homily-production
./backend/scripts/deploy-production.sh <commit-sha>
```

### Environment File Synchronization

```bash
# From your local machine
./backend/scripts/sync-env-files.sh stage      # Sync only stage
./backend/scripts/sync-env-files.sh production # Sync only production
./backend/scripts/sync-env-files.sh both       # Sync both (default)
```

## 📊 Staged Deployment Benefits

### Expected Downtime:

- **Previous setup**: 2-5 seconds (service restart gap)
- **Staged setup**: < 100ms (just port switch time) + coordination delays
- **Container cleanup**: 15-42 seconds for graceful container replacement

### How It Works:

1. **Backend deploys first** with zero-downtime
2. **60+ second stabilization** ensures backend is fully ready
3. **Frontend deploys second** only after backend is confirmed stable
4. **No breaking changes** - frontend always has working backend

### Deployment Timeline:

```
0s    - Backend deployment starts
30s   - Backend deployment completes (including container cleanup)
60s   - Backend stabilization period ends
90s   - Frontend deployment starts
210s  - Frontend deployment completes (Netlify build + publish)
```

### Container Management:

- **Graceful cleanup**: Existing containers are stopped gracefully before removal
- **Volume preservation**: Database volumes are preserved, only Redis cache is cleared
- **Health checks**: New containers must pass health checks before proceeding

## 📊 Monitoring and Management

### Service Management

```bash
# Stage environment
sudo systemctl status psychic-homily-stage
sudo journalctl -u psychic-homily-stage -f

# Production environment
sudo systemctl status psychic-homily-production
sudo journalctl -u psychic-homily-production -f

# Check Docker services
docker compose -f backend/docker-compose.stage.yml --env-file backend/.env.stage ps
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production ps

# View Docker service logs
docker compose -f backend/docker-compose.stage.yml --env-file backend/.env.stage logs -f db
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production logs -f redis
```

### Health Checks

```bash
# Stage environment
curl http://localhost:8081/health
docker compose -f backend/docker-compose.stage.yml --env-file backend/.env.stage exec -T db pg_isready -U stage_user -d stage_db

# Production environment
curl http://localhost:8080/health
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production exec -T db pg_isready -U prodpsychicadmin -d prodpsychicdb

# Redis health
docker compose -f backend/docker-compose.stage.yml --env-file backend/.env.stage exec redis redis-cli ping
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production exec redis redis-cli ping
```

## 🔒 Security Features

- **Non-root user**: Go applications run as `deploy` user
- **Systemd security**: Restricted file system access
- **Container isolation**: Database and Redis in isolated containers
- **No build tools**: Production environment has no compilation tools
- **Graceful shutdown**: Proper signal handling for clean restarts
- **Environment isolation**: Separate stage and production environments
- **Manual production triggers**: Production deployments require explicit confirmation

## 📈 Performance Benefits

- **Native Go binary**: Maximum performance, no container overhead
- **Static linking**: Single executable with no external dependencies
- **Optimized compilation**: Stripped binary with performance flags
- **Efficient resource usage**: Lower memory and CPU overhead
- **Zero-downtime**: Continuous service availability
- **Coordinated releases**: Frontend and backend stay in sync

## 🚨 Troubleshooting

### Common Issues

1. **Permission denied**: Ensure binary is executable and owned by correct user
2. **Port already in use**: Check if another service is using the required port
3. **Database connection failed**: Verify Docker services are running and healthy
4. **Service won't start**: Check systemd logs with `journalctl -u psychic-homily-stage` or `psychic-homily-production`
5. **Frontend deployment fails**: Check if backend is healthy first
6. **Container naming conflicts**: Scripts now handle graceful container cleanup
7. **Environment variables not loading**: Use `--env-file` flag with Docker Compose commands
8. **Hostname resolution errors**: Ensure DATABASE_URL uses `localhost`, not `db`

### Debug Commands

```bash
# Check service configuration
sudo systemctl cat psychic-homily-stage
sudo systemctl cat psychic-homily-production

# Test service configuration
sudo systemctl status psychic-homily-stage
sudo systemctl status psychic-homily-production

# View recent logs
sudo journalctl -u psychic-homily-stage --no-pager -n 50
sudo journalctl -u psychic-homily-production --no-pager -n 50

# Check file permissions
ls -la /opt/psychic-homily-stage/
ls -la /opt/psychic-homily-production/

# Check temporary app logs
tail -f /tmp/new-stage-app.log
tail -f /tmp/new-production-app.log

# Check Docker container status
docker compose -f backend/docker-compose.stage.yml --env-file backend/.env.stage ps
docker compose -f backend/docker-compose.prod.yml --env-file backend/.env.production ps
```

## Rollback Process

```bash
# Stage environment rollback
sudo systemctl stop psychic-homily-stage
cp backend/backups/psychic-homily-stage.backup.YYYYMMDD_HHMMSS psychic-homily-stage
sudo systemctl start psychic-homily-stage
curl http://localhost:8081/health

# Production environment rollback
sudo systemctl stop psychic-homily-production
cp backend/backups/psychic-homily-production.backup.YYYYMMDD_HHMMSS psychic-homily-production
sudo systemctl start psychic-homily-production
curl http://localhost:8080/health
```

**Note**: Rollback is automatic if health checks fail during deployment.

## Next Steps

1. **Set up GitHub secrets** for automated deployment:

   - `VPS_HOST`, `VPS_USERNAME`, `VPS_SSH_KEY` (stage)
   - `PROD_VPS_HOST`, `PROD_VPS_USERNAME`, `PROD_VPS_SSH_KEY` (production)
   - `NETLIFY_TOKEN`, `NETLIFY_SITE_ID` (stage frontend)
   - `NETLIFY_PROD_TOKEN`, `NETLIFY_PROD_SITE_ID` (production frontend)
   - `NETLIFY_STAGE_WEBHOOK`, `NETLIFY_PRODUCTION_WEBHOOK`

2. **Configure your VPS** with the required software
3. **Test stage deployment** with a small change (automatic on push to main)
4. **Test production deployment** manually via GitHub Actions
5. **Monitor logs and performance** for both environments
6. **Set up monitoring and alerting** (optional)

## 🏆 Why This Approach is Industry Standard

- **Netflix, Google, Uber**: All use similar staged deployment strategies
- **Go Community**: Prefers binary deployment for production
- **Small Projects**: Can achieve enterprise-grade deployment quality
- **Cost Effective**: No additional infrastructure needed
- **Professional**: Same reliability as large-scale deployments
- **Coordinated**: Frontend and backend deployments are properly sequenced
- **Multi-Environment**: Stage and production environments with proper isolation
- **Zero-Downtime**: Graceful container management and health-checked deployments
- **Automated Rollback**: Built-in safety mechanisms for failed deployments
