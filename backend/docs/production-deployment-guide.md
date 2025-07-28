# Production Deployment Guide

## Overview

This guide provides step-by-step instructions for safely deploying the Psychic Homily backend to production. The deployment process includes automatic backups, data safety checks, and rollback procedures.

## 🎯 Prerequisites

### Server Requirements

- **OS**: Ubuntu 20.04+ or similar Linux distribution
- **RAM**: Minimum 2GB (4GB recommended)
- **Storage**: 20GB+ available space
- **Network**: Public IP with ports 80/443 accessible

### Required Accounts & Services

- **Domain**: `psychichomily.com` (or your domain)
- **OAuth Providers**: Google, GitHub, Instagram developer accounts
- **Google Cloud Storage**: For automated backups
- **SSL Certificate**: Let's Encrypt or similar

### Local Development Setup

- **Git**: Latest version
- **SSH Access**: To your production server
- **Docker**: Installed on production server

## 🚀 Step 1: Server Preparation

### 1.1 Server Access

```bash
# SSH into your production server
ssh root@your-server-ip

# Update system packages
apt update && apt upgrade -y

# Install essential packages
apt install -y curl wget git nano htop
```

### 1.2 Install Docker & Docker Compose

```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Add user to docker group (if not root)
usermod -aG docker $USER

# Install Docker Compose
apt install -y docker-compose-plugin

# Verify installation
docker --version
docker compose version
```

### 1.3 Install Google Cloud CLI (for backups)

```bash
# Install gcloud CLI
curl https://sdk.cloud.google.com | bash
exec -l $SHELL

# Authenticate with Google Cloud
gcloud auth login
gcloud config set project your-project-id
```

## 🚀 Step 2: Application Setup

### 2.1 Clone Repository

```bash
# Navigate to application directory
cd /opt
git clone https://github.com/your-username/psychic-homily-web.git
cd psychic-homily-web/backend

# Set proper permissions
chown -R $USER:$USER /opt/psychic-homily-web
```

### 2.2 Create Production Environment

```bash
# Copy environment template
cp .env.example .env.production

# Edit production environment
nano .env.production
```

### 2.3 Production Environment Configuration

Fill in your `.env.production` with these values:

```bash
# Application Configuration
NODE_ENV=production
API_ADDR=0.0.0.0:8080
API_PORT=8080

# Database Configuration
DATABASE_URL=postgres://psychicadmin:${PROD_DB_PASSWORD}@localhost:5432/psychicdb_prod
POSTGRES_USER=psychicadmin
POSTGRES_PASSWORD=${PROD_DB_PASSWORD}  # Will be auto-generated
POSTGRES_DB=psychicdb_prod
POSTGRES_HOST=db
POSTGRES_PORT=5432

# CORS Configuration
CORS_ALLOWED_ORIGINS=https://psychichomily.com,https://www.psychichomily.com
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization
CORS_ALLOW_CREDENTIALS=true

# OAuth Configuration
GOOGLE_CLIENT_ID=your_production_google_client_id
GOOGLE_CLIENT_SECRET=your_production_google_client_secret
GITHUB_CLIENT_ID=your_production_github_client_id
GITHUB_CLIENT_SECRET=your_production_github_client_secret
INSTAGRAM_CLIENT_ID=your_production_instagram_client_id
INSTAGRAM_CLIENT_SECRET=your_production_instagram_client_secret
OAUTH_REDIRECT_URL=https://api.psychichomily.com/auth/callback
OAUTH_SECRET_KEY=your_production_secret_key_32_chars_long

# Google Cloud Storage (for backups)
GCS_BUCKET=psychic-homily-backups
```

## 🚀 Step 3: OAuth Provider Setup

### 3.1 Google OAuth

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing
3. Enable Google+ API
4. Create OAuth 2.0 credentials
5. Add authorized redirect URI: `https://api.psychichomily.com/auth/callback`
6. Copy Client ID and Secret to `.env.production`

### 3.2 GitHub OAuth

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Create new OAuth App
3. Set Authorization callback URL: `https://api.psychichomily.com/auth/callback`
4. Copy Client ID and Secret to `.env.production`

### 3.3 Instagram OAuth

1. Go to [Facebook Developers](https://developers.facebook.com/)
2. Create Instagram Basic Display app
3. Add redirect URI: `https://api.psychichomily.com/auth/callback`
4. Copy App ID and Secret to `.env.production`

## 🚀 Step 4: DNS Configuration

### 4.1 Domain Setup

Before setting up SSL, you need to configure DNS for the API subdomain:

1. **Log into your domain registrar** (where you purchased `psychichomily.com`)
2. **Add DNS record** for the API subdomain:

   - **Type**: A Record
   - **Name**: `api`
   - **Value**: Your DigitalOcean droplet's IP address
   - **TTL**: 300 (or default)

3. **Verify DNS propagation**:

   ```bash
   # Check if DNS is resolving
   nslookup api.psychichomily.com
   dig api.psychichomily.com
   ```

4. **Wait for propagation** (can take up to 24 hours, usually much faster)

### 4.2 SSL Certificate Setup

### 4.3 Install Certbot

```bash
# Install Certbot
apt install -y certbot

# Get SSL certificate
certbot certonly --standalone -d api.psychichomily.com

# Set up auto-renewal
crontab -e
# Add: 0 12 * * * /usr/bin/certbot renew --quiet
```

### 4.4 Configure Nginx (Required)

Nginx is required as a reverse proxy for the API subdomain:

```bash
# Install nginx
apt install -y nginx

# Create nginx configuration
nano /etc/nginx/sites-available/psychichomily
```

Nginx configuration:

```nginx
server {
    listen 80;
    server_name api.psychichomily.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.psychichomily.com;

    ssl_certificate /etc/letsencrypt/live/api.psychichomily.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.psychichomily.com/privkey.pem;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;
    add_header Content-Security-Policy "default-src 'self' http: https: data: blob: 'unsafe-inline'" always;

    # CORS headers for frontend
    add_header Access-Control-Allow-Origin "https://psychichomily.com" always;
    add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
    add_header Access-Control-Allow-Headers "Accept, Authorization, Content-Type, X-Requested-With" always;
    add_header Access-Control-Allow-Credentials "true" always;

    # Handle preflight requests
    if ($request_method = 'OPTIONS') {
        add_header Access-Control-Allow-Origin "https://psychichomily.com" always;
        add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Accept, Authorization, Content-Type, X-Requested-With" always;
        add_header Access-Control-Allow-Credentials "true" always;
        add_header Content-Length 0;
        add_header Content-Type text/plain;
        return 204;
    }

    # Proxy all requests to the Go API
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $server_name;
    }
}
```

```bash
# Enable site
ln -s /etc/nginx/sites-available/psychichomily /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx
```

## 🚀 Step 5: Initial Deployment

### 5.1 Make Scripts Executable

```bash
# Make deployment scripts executable
chmod +x scripts/deploy.sh
chmod +x scripts/backup.sh
chmod +x scripts/restore.sh
chmod +x scripts/verify.sh
```

### 5.2 Run Initial Deployment

```bash
# Run the initial deployment script
./scripts/deploy.sh --initial
```

### 5.3 Verify Deployment

```bash
# Check if services are running
docker compose -f docker-compose.prod.yml ps

# Test API health
curl https://api.psychichomily.com/health

# Check logs
docker compose -f docker-compose.prod.yml logs -f api
```

## 🚀 Step 6: Post-Deployment Verification

### 6.1 API Endpoints Test

```bash
# Health check
curl https://api.psychichomily.com/health

# OAuth login (should return success)
curl -X POST https://api.psychichomily.com/auth/login \
  -H "Content-Type: application/json" \
  -d '{"provider":"google"}'

# Show submission
curl -X POST https://api.psychichomily.com/show \
  -H "Content-Type: application/json" \
  -d '{
    "artists": [{"name": "Test Artist"}],
    "venue": "Test Venue",
    "date": "2025-01-15",
    "cost": "10",
    "ages": "18+",
    "city": "Test City",
    "state": "AZ"
  }'
```

### 6.2 Database Verification

```bash
# Connect to database
docker compose -f docker-compose.prod.yml exec db psql -U $POSTGRES_USER -d $POSTGRES_DB

# Check tables
\dt

# Check user tables
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM oauth_accounts;
```

### 6.3 OAuth Provider Test

1. Visit your website
2. Try logging in with each OAuth provider
3. Verify callback URLs are working
4. Check user creation in database

## 🚀 Step 7: Monitoring & Maintenance

### 7.1 Set Up Log Monitoring

```bash
# View real-time logs
docker compose -f docker-compose.prod.yml logs -f

# Check specific service logs
docker compose -f docker-compose.prod.yml logs api
docker compose -f docker-compose.prod.yml logs db
```

### 7.2 Automated Backups

```bash
# Set up daily backups
crontab -e

# Add daily backup at 2 AM (with GCS upload)
0 2 * * * cd /opt/psychic-homily-web/backend && ./scripts/backup.sh --upload >> /var/log/backup.log 2>&1

# Add system health check every 30 minutes
*/30 * * * * cd /opt/psychic-homily-web/backend && ./scripts/verify.sh --system >> /var/log/health.log 2>&1
```

### 7.3 Health Monitoring

```bash
# Create health check script
nano /opt/health-check.sh
```

Health check script:

```bash
#!/bin/bash
if ! curl -f https://api.psychichomily.com/health >/dev/null 2>&1; then
    echo "API is down! Sending alert..."
    # Add your alert mechanism here
fi
```

```bash
# Make executable and add to cron
chmod +x /opt/health-check.sh
crontab -e
# Add: */5 * * * * /opt/health-check.sh
```

## 🚀 Step 8: Updates & Maintenance

### 8.1 Code Updates

```bash
# Pull latest code
cd /opt/psychic-homily-web/backend
git pull origin main

# Run update deployment
./scripts/deploy.sh --update
```

### 8.2 Database Migrations

```bash
# Check migration status
docker compose -f docker-compose.prod.yml run --rm migrate -path /migrations -database "${DATABASE_URL}?sslmode=disable" version

# Run migrations manually (if needed)
docker compose -f docker-compose.prod.yml run --rm migrate
```

### 8.3 Backup Verification

```bash
# Verify system and backups
./scripts/verify.sh

# List available backups
ls -la backups/

# List GCS backups
gsutil ls gs://$GCS_BUCKET/backups/
```

## 🚨 Emergency Procedures

### Rollback Deployment

```bash
# Stop services
docker compose -f docker-compose.prod.yml down

# Restore from backup (local or GCS)
./scripts/restore.sh backup_YYYYMMDD_HHMMSS.sql
# OR for GCS: ./scripts/restore.sh backup_YYYYMMDD_HHMMSS.sql --from-gcs

# Restart with previous version
git checkout previous-commit
./scripts/deploy.sh --update
```

### Database Recovery

```bash
# Stop API service
docker compose -f docker-compose.prod.yml stop api

# Restore database
docker compose -f docker-compose.prod.yml exec -T db psql -U $POSTGRES_USER -d $POSTGRES_DB < backup_file.sql

# Restart API
docker compose -f docker-compose.prod.yml start api
```

### Complete Server Recovery

```bash
# If server is completely down
ssh root@your-server-ip

# Reinstall Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Clone repository
cd /opt
git clone https://github.com/your-username/psychic-homily-web.git
cd psychic-homily-web/backend

# Restore from latest backup
./scripts/restore.sh backup_YYYYMMDD_HHMMSS.sql --from-gcs

# Deploy
./scripts/deploy.sh --initial
```

## 📊 Monitoring Checklist

### Daily Checks

- [ ] API health endpoint responding
- [ ] Database connection working
- [ ] OAuth providers accessible
- [ ] Backup completed successfully

### Weekly Checks

- [ ] Review application logs
- [ ] Check disk space usage
- [ ] Verify SSL certificate validity
- [ ] Test backup restoration

### Monthly Checks

- [ ] Update system packages
- [ ] Review security logs
- [ ] Check OAuth provider quotas
- [ ] Verify monitoring alerts

## 🔧 Troubleshooting

### Common Issues

**API not responding:**

```bash
# Check container status
docker compose -f docker-compose.prod.yml ps

# Check logs
docker compose -f docker-compose.prod.yml logs api

# Restart API
docker compose -f docker-compose.prod.yml restart api
```

**Database connection issues:**

```bash
# Check database health
docker compose -f docker-compose.prod.yml exec db pg_isready -U $POSTGRES_USER

# Check database logs
docker compose -f docker-compose.prod.yml logs db

# Restart database
docker compose -f docker-compose.prod.yml restart db
```

**OAuth callback errors:**

1. Verify redirect URLs in OAuth provider settings
2. Check environment variables
3. Ensure HTTPS is working
4. Review application logs for specific errors

**Backup failures:**

```bash
# Check GCS authentication
gcloud auth application-default login

# Verify bucket exists
gsutil ls gs://psychic-homily-backups/

# Test backup manually
./scripts/backup.sh --upload

# Verify system health
./scripts/verify.sh --backups
```

## 📞 Support

If you encounter issues:

1. **Check logs**: `docker compose -f docker-compose.prod.yml logs`
2. **Verify configuration**: Check `.env.production` values
3. **Test connectivity**: Ensure ports and DNS are correct
4. **Review documentation**: Check this guide and API documentation

## 🎯 Success Metrics

After deployment, verify:

- [ ] API responds to health checks
- [ ] OAuth login works for all providers
- [ ] Database migrations completed
- [ ] Backups are being created
- [ ] SSL certificate is valid
- [ ] Monitoring is active
- [ ] Logs are being generated

Your production deployment is now complete and secure! 🚀
