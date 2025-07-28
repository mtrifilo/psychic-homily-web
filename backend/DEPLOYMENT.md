# DigitalOcean Deployment Checklist

This checklist covers everything you need to deploy your Go backend to DigitalOcean with `api.psychichomily.com`.

## **🌐 Domain Configuration**

### **1. DNS Setup**

- [ ] Add A record for `api.psychichomily.com` pointing to your DigitalOcean droplet IP
- [ ] Verify DNS propagation: `dig api.psychichomily.com`
- [ ] Test subdomain resolves: `ping api.psychichomily.com`

### **2. SSL Certificate**

- [ ] Install Certbot on DigitalOcean droplet
- [ ] Obtain SSL certificate: `certbot certonly --nginx -d api.psychichomily.com`
- [ ] Set up auto-renewal: `crontab -e` (add certbot renewal)
- [ ] Test SSL: `curl -I https://api.psychichomily.com/health`

## **🖥️ DigitalOcean Droplet Setup**

### **1. Create Droplet**

- [ ] Create Ubuntu 22.04 LTS droplet
- [ ] Choose appropriate size (2GB RAM minimum recommended)
- [ ] Add SSH key for secure access
- [ ] Note the droplet IP address

### **2. Initial Server Setup**

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install essential packages
sudo apt install -y curl wget git unzip software-properties-common

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# Install Nginx
sudo apt install -y nginx

# Install Certbot
sudo apt install -y certbot python3-certbot-nginx
```

### **3. Database Setup**

- [ ] **Option A**: Use DigitalOcean Managed Database (recommended)
  - [ ] Create PostgreSQL database cluster
  - [ ] Note connection string and credentials
  - [ ] Configure firewall rules to allow droplet access
- [ ] **Option B**: Install PostgreSQL locally
  - [ ] `sudo apt install -y postgresql postgresql-contrib`
  - [ ] Configure PostgreSQL for external connections
  - [ ] Create database and user

## **🔧 Application Deployment**

### **1. Clone and Setup Application**

```bash
# Clone your repository
git clone https://github.com/yourusername/psychic-homily-web.git
cd psychic-homily-web/backend

# Create production environment file
cp env.production.example .env.production
nano .env.production  # Edit with your actual values
```

### **2. Configure Environment Variables**

```bash
# Generate secure keys
openssl rand -base64 32  # For OAUTH_SECRET_KEY
openssl rand -base64 32  # For JWT_SECRET_KEY

# Edit .env.production with:
# - Database connection string
# - OAuth provider credentials
# - Generated secret keys
# - Correct redirect URLs
```

### **3. Deploy Application**

```bash
# Make deployment script executable
chmod +x deploy.sh

# Run deployment
./deploy.sh

# Check application status
docker-compose -f docker-compose.prod.yml ps
docker-compose -f docker-compose.prod.yml logs app
```

## **🌐 Nginx Configuration**

### **1. Configure Nginx**

```bash
# Copy nginx configuration
sudo cp nginx.conf /etc/nginx/sites-available/api.psychichomily.com

# Enable site
sudo ln -s /etc/nginx/sites-available/api.psychichomily.com /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload nginx
sudo systemctl reload nginx
```

### **2. SSL Configuration**

```bash
# Obtain SSL certificate
sudo certbot --nginx -d api.psychichomily.com

# Test auto-renewal
sudo certbot renew --dry-run
```

## **🔐 OAuth Provider Configuration**

### **1. Google OAuth**

- [ ] Go to Google Cloud Console
- [ ] Add `https://api.psychichomily.com/auth/callback` to authorized redirect URIs
- [ ] Update client ID and secret in `.env.production`

### **2. GitHub OAuth**

- [ ] Go to GitHub Developer Settings
- [ ] Add `https://api.psychichomily.com/auth/callback` to callback URL
- [ ] Update client ID and secret in `.env.production`

### **3. Instagram OAuth**

- [ ] Go to Facebook Developer Console
- [ ] Add `https://api.psychichomily.com/auth/callback` to valid OAuth redirect URIs
- [ ] Update client ID and secret in `.env.production`

## **🧪 Testing**

### **1. Health Check**

```bash
# Test health endpoint
curl https://api.psychichomily.com/health

# Expected response: {"status":"ok"}
```

### **2. OAuth Flow Test**

```bash
# Test OAuth login redirect
curl -I https://api.psychichomily.com/auth/login/google

# Should return 302 redirect to Google OAuth
```

### **3. CORS Test**

```bash
# Test CORS headers
curl -H "Origin: https://psychichomily.com" \
     -H "Access-Control-Request-Method: POST" \
     -H "Access-Control-Request-Headers: Content-Type" \
     -X OPTIONS \
     https://api.psychichomily.com/auth/profile
```

## **📊 Monitoring and Logs**

### **1. Application Logs**

```bash
# View application logs
docker-compose -f docker-compose.prod.yml logs -f app

# View nginx logs
sudo tail -f /var/log/nginx/api.psychichomily.com.access.log
sudo tail -f /var/log/nginx/api.psychichomily.com.error.log
```

### **2. System Monitoring**

```bash
# Check system resources
htop
df -h
docker system df

# Check application health
curl https://api.psychichomily.com/health
```

## **🔄 Continuous Deployment**

### **1. Setup Git Hooks (Optional)**

```bash
# Create deployment script
cat > /root/deploy.sh << 'EOF'
#!/bin/bash
cd /root/psychic-homily-web/backend
git pull origin main
./deploy.sh
EOF

chmod +x /root/deploy.sh
```

### **2. GitHub Actions (Optional)**

Create `.github/workflows/deploy.yml`:

```yaml
name: Deploy to DigitalOcean
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Deploy to server
        uses: appleboy/ssh-action@v0.1.4
        with:
          host: ${{ secrets.DROPLET_IP }}
          username: ${{ secrets.DROPLET_USER }}
          key: ${{ secrets.DROPLET_SSH_KEY }}
          script: |
            cd /root/psychic-homily-web/backend
            git pull origin main
            ./deploy.sh
```

## **🛡️ Security Checklist**

### **1. Firewall Configuration**

```bash
# Configure UFW firewall
sudo ufw allow ssh
sudo ufw allow 80
sudo ufw allow 443
sudo ufw enable
```

### **2. Security Headers**

- [ ] Verify Nginx security headers are set
- [ ] Test HTTPS redirects work
- [ ] Confirm CORS is properly configured

### **3. Environment Security**

- [ ] All secrets are in `.env.production` (not in code)
- [ ] `.env.production` has restricted permissions: `chmod 600 .env.production`
- [ ] Database connection uses SSL
- [ ] JWT secret is cryptographically secure

## **📈 Performance Optimization**

### **1. Nginx Optimization**

- [ ] Enable gzip compression
- [ ] Configure caching headers
- [ ] Optimize worker processes

### **2. Application Optimization**

- [ ] Monitor memory usage
- [ ] Configure appropriate JWT expiry
- [ ] Set up database connection pooling

## **🚨 Troubleshooting**

### **Common Issues**

1. **CORS Errors**

   - Check CORS configuration in `config.go`
   - Verify frontend origin is in allowed origins

2. **OAuth Redirect Errors**

   - Verify redirect URL in OAuth provider settings
   - Check SSL certificate is valid

3. **Database Connection Issues**

   - Verify database connection string
   - Check firewall rules for database access

4. **JWT Token Issues**
   - Verify JWT secret is set correctly
   - Check token expiration settings

### **Useful Commands**

```bash
# Restart application
docker-compose -f docker-compose.prod.yml restart

# View logs
docker-compose -f docker-compose.prod.yml logs app

# Check nginx status
sudo systemctl status nginx

# Test SSL certificate
openssl s_client -connect api.psychichomily.com:443 -servername api.psychichomily.com
```

## **✅ Final Verification**

- [ ] `https://api.psychichomily.com/health` returns 200 OK
- [ ] OAuth login flow works from frontend
- [ ] JWT tokens are generated and validated
- [ ] Protected endpoints require authentication
- [ ] CORS allows frontend requests
- [ ] SSL certificate is valid and auto-renewing
- [ ] Application logs show no errors
- [ ] Database migrations completed successfully
