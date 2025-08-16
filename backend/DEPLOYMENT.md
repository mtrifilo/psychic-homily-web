# Backend Deployment Guide (Staged Zero-Downtime)

This guide explains how to deploy the Psychic Homily backend using staged zero-downtime deployment with Go binaries and Docker infrastructure services.

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

### 2. Create Application Directory

```bash
sudo mkdir -p /opt/psychic-homily-backend
sudo chown $USER:$USER /opt/psychic-homily-backend
cd /opt/psychic-homily-backend
```

### 3. Set Up Systemd Service

```bash
# Copy the service file
sudo cp systemd/psychic-homily-backend.service /etc/systemd/system/

# Create the service user
sudo useradd -r -s /bin/false psychic-homily

# Reload systemd
sudo systemctl daemon-reload
sudo systemctl enable psychic-homily-backend

# Start the service
sudo systemctl start psychic-homily-backend

# Check status
sudo systemctl status psychic-homily-backend
```

### 4. Configure Environment Variables

Create `.env.production` with your production settings:

```bash
# Database
POSTGRES_USER=your_db_user
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=psychic_homily_prod

# API
API_ADDR=0.0.0.0:8080
JWT_SECRET_KEY=your_jwt_secret

# OAuth (if using)
GITHUB_CLIENT_ID=your_github_client_id
GITHUB_CLIENT_SECRET=your_github_client_secret
```

## Deployment Process

### Automated Staged Deployment (GitHub Actions)

1. **Push to main branch** with changes
2. **GitHub Actions** automatically:
   - **Stage 1**: Deploys backend if backend files changed
   - **Wait Period**: 60+ seconds for backend stabilization
   - **Stage 2**: Deploys frontend if frontend files changed

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
# On your VPS
cd /opt/psychic-homily-backend
./scripts/deploy-zero-downtime.sh <commit-sha>
```

## 📊 Staged Deployment Benefits

### Expected Downtime:

- **Previous setup**: 2-5 seconds (service restart gap)
- **Staged setup**: < 100ms (just port switch time) + coordination delays

### How It Works:

1. **Backend deploys first** with zero-downtime
2. **60+ second stabilization** ensures backend is fully ready
3. **Frontend deploys second** only after backend is confirmed stable
4. **No breaking changes** - frontend always has working backend

### Deployment Timeline:

```
0s    - Backend deployment starts
30s   - Backend deployment completes
60s   - Backend stabilization period ends
90s   - Frontend deployment starts
210s  - Frontend deployment completes
```

## 📊 Monitoring and Management

### Service Management

```bash
# Check Go application status
sudo systemctl status psychic-homily-backend

# View Go application logs
sudo journalctl -u psychic-homily-backend -f

# Check Docker services
docker-compose -f docker-compose.prod.yml ps

# View Docker service logs
docker-compose -f docker-compose.prod.yml logs -f db
docker-compose -f docker-compose.prod.yml logs -f redis
```

### Health Checks

```bash
# Application health
curl http://localhost:8080/health

# Database health
docker-compose -f docker-compose.prod.yml exec db pg_isready -U your_user -d your_db

# Redis health
docker-compose -f docker-compose.prod.yml exec redis redis-cli ping
```

## 🔒 Security Features

- **Non-root user**: Go application runs as `psychic-homily` user
- **Systemd security**: Restricted file system access
- **Container isolation**: Database and Redis in isolated containers
- **No build tools**: Production environment has no compilation tools
- **Graceful shutdown**: Proper signal handling for clean restarts

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
2. **Port already in use**: Check if another service is using port 8080
3. **Database connection failed**: Verify Docker services are running and healthy
4. **Service won't start**: Check systemd logs with `journalctl -u psychic-homily-backend`
5. **Frontend deployment fails**: Check if backend is healthy first

### Debug Commands

```bash
# Check service configuration
sudo systemctl cat psychic-homily-backend

# Test service configuration
sudo systemctl status psychic-homily-backend

# View recent logs
sudo journalctl -u psychic-homily-backend --no-pager -n 50

# Check file permissions
ls -la /opt/psychic-homily-backend/

# Check temporary app logs
tail -f /tmp/new-app.log
```

## Rollback Process

```bash
# Stop current service
sudo systemctl stop psychic-homily-backend

# Restore previous binary
cp backups/psychic-homily-backend.backup.YYYYMMDD_HHMMSS psychic-homily-backend

# Start service
sudo systemctl start psychic-homily-backend

# Verify health
curl http://localhost:8080/health
```

## Next Steps

1. Set up GitHub secrets for automated deployment
2. Configure your VPS with the required software
3. Test the staged deployment with a small change
4. Monitor logs and performance
5. Set up monitoring and alerting (optional)

## 🏆 Why This Approach is Industry Standard

- **Netflix, Google, Uber**: All use similar staged deployment strategies
- **Go Community**: Prefers binary deployment for production
- **Small Projects**: Can achieve enterprise-grade deployment quality
- **Cost Effective**: No additional infrastructure needed
- **Professional**: Same reliability as large-scale deployments
- **Coordinated**: Frontend and backend deployments are properly sequenced
