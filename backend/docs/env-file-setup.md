```bash
# Generate 32-byte keys (44 characters in base64)
openssl rand -base64 32

# Generate multiple keys for different purposes
echo "OAUTH_SECRET_KEY=$(openssl rand -base64 32)"
echo "JWT_SECRET_KEY=$(openssl rand -base64 32)"
echo "SESSION_SECRET=$(openssl rand -base64 32)"
```

## Gitignore

# Never commit production secrets

.env.production
.env.\*.production
.env.local

# Database backups may contain sensitive data

backups/
_.sql
_.dump
