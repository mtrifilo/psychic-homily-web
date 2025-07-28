# Server setup

# On your Digital Ocean droplet

# 1. Install Docker and Docker Compose

curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# 2. Install Nginx

sudo apt update
sudo apt install nginx certbot python3-certbot-nginx

# 3. Clone your repository

git clone https://github.com/yourusername/psychic-homily-web.git
cd psychic-homily-web/backend

# 4. Set up SSL certificate

sudo certbot --nginx -d psychichomily.com -d www.psychichomily.com

# 5. Deploy

chmod +x deploy.sh
./deploy.sh

---

# Simple monitoring

# Simple health check

curl -f https://psychichomily.com/health || echo "API is down!"

# Check disk space

df -h

# Check memory usage

free -h

# Check running containers

docker ps

---

Backups

# Check recent backups

gsutil ls -l gs://psychic-homily-backups/backups/ | tail -10

# Check backup log

tail -f /var/log/backup.log

# Get backup size

gsutil du -sh gs://psychic-homily-backups/backups/

# Set up daily backups at 2 AM

crontab -e

# Add this line:

0 2 \* \* \* /path/to/your/project/backend/backup.sh >> /var/log/backup.log 2>&1
