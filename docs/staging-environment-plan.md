# Staging Environment Implementation Plan

## 🎯 **Project Overview**

**Goal**: Create a complete staging environment for testing changes before production deployment.

**Architecture**:

- **Frontend**: Hugo + React components hosted on Netlify
- **Backend**: Go API hosted on VPS
- **Frontend Staging**: `stage.psychichomily.com` (Netlify staging site)
- **Backend Staging**: `stage.api.psychichomily.com` (VPS staging backend)
- **Production Frontend**: `psychichomily.com` (Netlify production site)
- **Production Backend**: `api.psychichomily.com` (VPS production backend)

## 🏗️ **Infrastructure Setup**

### **Phase 1: VPS Staging Environment**

#### **1.1 Directory Structure Setup**

```bash
/opt/
├── psychic-homily-backend/           # Production (existing)
│   ├── psychic-homily-backend       # Production binary
│   ├── docker-compose.prod.yml      # Production services
│   └── scripts/
├── psychic-homily-staging/           # Staging (new)
│   ├── psychic-homily-staging       # Staging binary
│   ├── docker-compose.staging.yml   # Staging services
│   ├── scripts/
│   └── systemd/
└── shared/                           # Shared resources
    ├── ssl/
    └── nginx/
```

#### **1.2 Port Configuration**

```bash
# Production Backend
api.psychichomily.com:80/443 → Production backend (port 8080)
api.psychichomily.com:5432 → Production PostgreSQL
api.psychichomily.com:6379 → Production Redis

# Staging Backend
stage.api.psychichomily.com:80/443 → Staging backend (port 8081)
stage.api.psychichomily.com:5433 → Staging PostgreSQL
stage.api.psychichomily.com:6380 → Staging Redis
```

#### **1.3 Docker Services Setup**

- **Production**: `docker-compose.prod.yml` (existing)
- **Staging**: `docker-compose.staging.yml` (new)
- **Separate databases**: `psychic_homily_prod` vs `psychic_homily_staging`
- **Separate Redis instances**: Different ports and data directories

#### **1.4 Systemd Services**

- **Production**: `psychic-homily-backend.service` (existing)
- **Staging**: `psychic-homily-staging.service` (new)
- **Independent management**: Can start/stop staging without affecting production

### **Phase 2: DNS & Subdomain Configuration**

#### **2.1 DNS Records**

```bash
# A Records (Backend APIs)
api.psychichomily.com → 143.198.146.17 (production backend VPS)
stage.api.psychichomily.com → 143.198.146.17 (staging backend VPS)

# CNAME Records (Frontend - Netlify)
www.psychichomily.com → psychichomily.netlify.app (production)
www.stage.psychichomily.com → psychic-homily-staging.netlify.app (staging)
```

#### **2.2 Nginx Reverse Proxy**

- **Production Backend**: Routes `api.psychichomily.com` to port 8080
- **Staging Backend**: Routes `stage.api.psychichomily.com` to port 8081
- **SSL certificates**: Handle both backend subdomains
- **Health checks**: Monitor both backend instances

### **Phase 3: Netlify Staging Site**

#### **3.1 Staging Site Creation**

- **Site name**: `psychic-homily-staging`
- **Custom domain**: `stage.psychichomily.com`
- **Build settings**: Use staging Hugo configuration
- **Environment variables**: Staging API endpoints

#### **3.2 Hugo Configuration Files**

- **Staging**: `hugo.staging.toml`
- **Production**: `hugo.production.toml`
- **Environment-specific**: API URLs, base URLs, feature flags

## 🔄 **CI/CD Pipeline Updates**

### **Phase 4: GitHub Actions Workflows**

#### **4.1 Staging Deployment Workflow**

```yaml
# .github/workflows/deploy-staging.yml
name: Deploy to Staging

on:
  push:
    branches: [main]

jobs:
  deploy-backend-staging:
    # Deploy Go backend to VPS port 8081
    # Accessible at stage.api.psychichomily.com

  deploy-frontend-staging:
    needs: deploy-backend-staging
    # Build Hugo + React for staging
    # Deploy to Netlify staging site
    # Accessible at stage.psychichomily.com
```

#### **4.2 Production Deployment Workflow**

```yaml
# .github/workflows/deploy-production.yml
name: Deploy to Production

on:
  workflow_dispatch:
    inputs:
      confirm:
        description: "Type 'PRODUCTION' to confirm"
        required: true

jobs:
  deploy-backend-production:
    # Deploy Go backend to VPS port 8080
    # Accessible at api.psychichomily.com

  deploy-frontend-production:
    needs: deploy-backend-production
    # Build Hugo + React for production
    # Deploy to Netlify production site
    # Accessible at psychichomily.com
```

#### **4.3 Daily Build Workflow Updates**

- **Staging**: Daily builds to staging environment
- **Production**: Manual builds only
- **Environment detection**: Automatic Hugo config selection

### **Phase 5: Build Configuration Updates**

#### **5.1 Package.json Scripts**

```json
{
  "scripts": {
    "build:staging": "cd components && pnpm build:staging && cd .. && hugo --config hugo.staging.toml --gc --minify",
    "build:production": "cd components && pnpm build:production && cd .. && hugo --config hugo.production.toml --gc --minify"
  }
}
```

#### **5.2 Vite Configuration**

- **Staging build**: Output to `dist-staging/`
- **Production build**: Output to `dist/`
- **Environment variables**: Different API endpoints

#### **5.3 Hugo Environment Configs**

- **Staging**: `hugo.staging.toml`
- **Production**: `hugo.production.toml`
- **Base URLs**: Different domains for each environment

## 🗄️ **Database & Data Management**

### **Phase 6: Database Isolation**

#### **6.1 PostgreSQL Setup**

```bash
# Production
POSTGRES_DB=psychic_homily_prod
POSTGRES_USER=ph_prod_user
POSTGRES_PASSWORD=secure_prod_password

# Staging
POSTGRES_DB=psychic_homily_staging
POSTGRES_USER=ph_staging_user
POSTGRES_PASSWORD=secure_staging_password
```

#### **6.2 Redis Setup**

```bash
# Production
REDIS_DB=0
REDIS_PORT=6379

# Staging
REDIS_DB=1
REDIS_PORT=6380
```

#### **6.3 Data Seeding**

- **Production**: Real user data
- **Staging**: Test data, sample shows, mock users
- **Migration testing**: Test all database changes on staging first

### **Phase 7: Environment Configuration**

#### **7.1 Backend Environment Files**

```bash
# .env.staging
API_ADDR=0.0.0.0:8081
POSTGRES_DB=psychic_homily_staging
POSTGRES_USER=ph_staging_user
POSTGRES_PASSWORD=secure_staging_password
REDIS_DB=1
REDIS_PORT=6380

# .env.production
API_ADDR=0.0.0.0:8080
POSTGRES_DB=psychic_homily_prod
POSTGRES_USER=ph_prod_user
POSTGRES_PASSWORD=secure_prod_password
REDIS_DB=0
REDIS_PORT=6379
```

#### **7.2 Frontend Environment Variables**

```bash
# Netlify Staging
ENVIRONMENT=stage
REACT_APP_API_URL=https://stage.api.psychichomily.com

# Netlify Production
ENVIRONMENT=production
REACT_APP_API_URL=https://api.psychichomily.com
```

**Note**: We now use a single `ENVIRONMENT` variable as the source of truth. All other environment variables (HUGO_ENV, REACT_APP_ENV, NODE_ENV) are automatically derived from this single variable. The staging environment uses `ENVIRONMENT=stage` to match the subdomain `stage.psychichomily.com`.

## 🚀 **Deployment Strategy**

### **Phase 8: Deployment Workflow**

#### **8.1 Staging Deployment (Automatic)**

1. **Push to main branch** → Triggers staging deployment
2. **Backend deploys** → Go API on VPS port 8081 (stage.api.psychichomily.com)
3. **Database migrations** → Run on staging database
4. **Frontend builds** → Hugo + React for staging
5. **Netlify deploys** → Staging site (stage.psychichomily.com) with staging API endpoints
6. **Health checks** → Verify staging environment

#### **8.2 Production Deployment (Manual)**

1. **Manual trigger** → Production deployment workflow
2. **Backend deploys** → Go API on VPS port 8080 (api.psychichomily.com)
3. **Database migrations** → Run on production database
4. **Frontend builds** → Hugo + React for production
5. **Netlify deploys** → Production site (psychichomily.com) with production API endpoints
6. **Health checks** → Verify production environment

#### **8.3 Rollback Strategy**

- **Backend rollback**: Restore previous binary from backups
- **Frontend rollback**: Netlify automatic rollback or manual trigger
- **Database rollback**: Restore from backups if needed

## 📊 **Monitoring & Health Checks**

### **Phase 9: Health Monitoring**

#### **9.1 Backend Health Checks**

```bash
# Production Backend
curl https://api.psychichomily.com/health

# Staging Backend
curl https://stage.api.psychichomily.com/health
```

#### **9.2 Database Health Checks**

```bash
# Production
docker-compose -f docker-compose.prod.yml exec db pg_isready

# Staging
docker-compose -f docker-compose.staging.yml exec db pg_isready
```

#### **9.3 Frontend Health Checks**

- **Production**: Monitor production Netlify site (psychichomily.com)
- **Staging**: Monitor staging Netlify site (stage.psychichomily.com)
- **Uptime monitoring**: Track both environments

## 🔒 **Security & Access Control**

### **Phase 10: Security Measures**

#### **10.1 Environment Isolation**

- **Separate databases**: No cross-environment data access
- **Separate Redis instances**: Isolated caching
- **Separate API keys**: Different OAuth credentials for staging
- **Separate subdomains**: Complete network isolation

#### **10.2 Access Control**

- **Staging access**: Limited to development team
- **Production access**: Restricted to authorized personnel
- **API rate limiting**: Different limits for staging vs production

#### **10.3 SSL & Certificates**

- **Production**: Full SSL with proper certificates
- **Staging**: SSL with staging certificates
- **Certificate management**: Handle both backend subdomains

## 📋 **Implementation Checklist**

### **Infrastructure Setup**

- [ ] Create staging directory structure on VPS
- [x] Set up staging Docker services
- [x] Configure staging systemd service
- [ ] Set up Nginx reverse proxy for staging backend
- [x] Configure DNS records for staging subdomains

### **Backend Configuration**

- [ ] Create staging environment files
- [x] Update Docker Compose for staging
- [x] Create staging deployment scripts
- [ ] Test staging backend deployment

### **Frontend Configuration**

- [x] Create Netlify staging site
- [x] Set up Hugo staging configuration
- [x] Update Vite build configuration
- [x] Test staging frontend deployment

### **CI/CD Updates**

- [x] Create staging deployment workflow
- [x] Create production deployment workflow
- [x] Update daily build workflow
- [ ] Test complete deployment pipeline

### **Testing & Validation**

- [ ] Test staging environment end-to-end
- [ ] Verify database isolation
- [ ] Test API endpoints on staging
- [ ] Validate frontend-backend communication

## 🎯 **Success Criteria**

### **Staging Environment**

- [ ] `stage.psychichomily.com` accessible and functional (Netlify)
- [ ] `stage.api.psychichomily.com` responding on port 8081 (VPS)
- [ ] Staging database isolated and functional
- [ ] Staging frontend displaying correctly
- [ ] React components working with staging API

### **Production Environment**

- [ ] `psychichomily.com` continues to work normally (Netlify)
- [ ] `api.psychichomily.com` responding on port 8080 (VPS)
- [ ] Production database unaffected by staging setup
- [ ] Production frontend unchanged

### **Deployment Pipeline**

- [x] Push to main triggers staging deployment
- [ ] Manual trigger deploys to production
- [ ] Both environments can run simultaneously
- [ ] Rollback procedures tested and working

## 📅 **Timeline Estimate**

- **Week 1**: Infrastructure setup (VPS, Docker, DNS) ✅ **COMPLETED**
- **Week 2**: Backend configuration and testing ✅ **COMPLETED**
- **Week 3**: Frontend configuration and testing ✅ **COMPLETED**
- **Week 4**: CI/CD updates and full validation ✅ **COMPLETED**

## 🚨 **Risk Mitigation**

### **High Risk Items**

- **Database migrations**: Always test on staging first
- **API changes**: Verify backward compatibility
- **SSL certificates**: Ensure proper renewal process

### **Medium Risk Items**

- **Port conflicts**: Verify no port overlaps
- **Resource contention**: Monitor VPS resource usage
- **DNS propagation**: Allow time for DNS changes

### **Low Risk Items**

- **Frontend styling**: Visual changes are low risk
- **Content updates**: Non-functional changes are safe

## 🔄 **Next Steps**

1. **✅ Review and approve** this updated plan
2. **✅ Set up VPS staging environment** (backend on port 8081)
3. **✅ Create Netlify staging site** with custom domain
4. **✅ Configure DNS records** for both staging subdomains
5. **✅ Update CI/CD workflows** for staging-first deployment
6. **🔄 Test complete staging environment**
7. **🔄 Validate production deployment process**

## 🔑 **Required GitHub Secrets Setup**

Before the deployment workflows can function, you must configure these GitHub repository secrets:

### **VPS Connection Secrets:**

```bash
VPS_HOST=143.198.146.17          # Your production VPS IP address
VPS_USERNAME=your_username       # SSH username for VPS access
VPS_SSH_KEY=your_private_key    # SSH private key for VPS authentication
```

### **Netlify Build Hook Secrets:**

```bash
NETLIFY_STAGE_WEBHOOK=your_staging_webhook_url      # Netlify staging site build hook
NETLIFY_PRODUCTION_WEBHOOK=your_production_webhook   # Netlify production site build hook
```

### **How to Set Up GitHub Secrets:**

1. **Go to your GitHub repository**
2. **Click "Settings" tab**
3. **Click "Secrets and variables" → "Actions"**
4. **Click "New repository secret"**
5. **Add each secret** with the exact name and value above

### **Getting Netlify Build Hooks:**

1. **For Staging Site:**

   - Go to Netlify staging site settings
   - Navigate to "Build & deploy" → "Build hooks"
   - Click "Add build hook"
   - Copy the generated URL

2. **For Production Site:**
   - Go to Netlify production site settings
   - Navigate to "Build & deploy" → "Build hooks"
   - Click "Add build hook"
   - Copy the generated URL

### **SSH Key Setup:**

1. **Generate SSH key pair** (if you don't have one):

   ```bash
   # Modern Ed25519 key (recommended)
   ssh-keygen -t ed25519 -C "your_email@example.com"

   # Alternative: RSA with 4096 bits (if Ed25519 not supported)
   ssh-keygen -t rsa -b 4096 -C "your_email@example.com"
   ```

2. **Add public key to VPS:**

   ```bash
   ssh-copy-id username@143.198.146.17
   ```

3. **Copy private key content** for the `VPS_SSH_KEY` secret:

   ```bash
   cat ~/.ssh/id_ed25519  # or id_rsa if using RSA
   ```

4. **Secure your SSH key** (recommended):

   ```bash
   # Set restrictive permissions
   chmod 600 ~/.ssh/id_ed25519
   chmod 700 ~/.ssh

   # Add to SSH agent for convenience
   ssh-add ~/.ssh/id_ed25519
   ```

### **Security Notes:**

- **Never commit secrets** to your repository
- **Use strong SSH keys** with passphrase protection
- **Rotate secrets regularly** for security
- **Limit VPS access** to only necessary operations

## 🎉 **What's Been Implemented:**

### **✅ Frontend Staging (Netlify):**

- Hugo staging configuration (`hugo.staging.toml`)
- Vite staging build configuration
- Staging build scripts in package.json
- Netlify staging context configuration

### **✅ Backend Staging (VPS):**

- Staging Docker Compose (`docker-compose.staging.yml`)
- Staging environment configuration (`env.staging`)
- Staging deployment script (`deploy-staging.sh`)
- Staging systemd service

### **✅ CI/CD Pipeline:**

- Staging deployment workflow (automatic on main)
- Production deployment workflow (manual trigger)
- Environment-specific builds and configurations

### **✅ Build Configuration:**

- Staging and production build scripts
- Environment-specific Vite configurations
- Hugo environment configurations

## 🚀 **Ready for Testing:**

Your staging environment is now fully configured and ready for testing! The next step is to:

1. **Test the staging deployment** by pushing to main
2. **Verify both environments** are working correctly
3. **Test the complete workflow** end-to-end

---

**Document Version**: 1.2  
**Last Updated**: [Current Date]  
**Next Review**: After testing and validation
