# PostgreSQL 17 → 18 Upgrade Guide

This guide covers the safe upgrade process for both **Stage** and **Production** environments.

## ⚠️ Important Notes

- **Downtime Required**: ~5-10 minutes for upgrade process
- **Backups Created**: SQL dump + filesystem backup (both kept for 30 days)
- **Rollback Available**: Can revert to PostgreSQL 17 if needed
- **New Features**: Includes autocomplete search indexes (`pg_trgm`)

## 📋 Prerequisites

- SSH access to the server
- Sudo privileges
- At least 20% free disk space
- Maintenance window scheduled (for production)

## 🔄 Upgrade Process

### Step 1: Pre-flight Check

Run the pre-flight check to ensure system is ready:

```bash
# SSH to server
ssh deploy@your-server

# Navigate to project
cd /opt/psychic-homily-<stage|production>

# Run pre-flight check
./backend/scripts/preflight-check-postgres-upgrade.sh <stage|production>
```

Expected output:

```
✅ All checks passed! System is ready for upgrade.
```

If any checks fail, fix those issues first.

### Step 2: Perform Upgrade

#### For Stage:

```bash
cd /opt/psychic-homily-stage
./backend/scripts/upgrade-postgres-to-18.sh stage
```

#### For Production:

```bash
cd /opt/psychic-homily-production

# Extra confirmation required
./backend/scripts/upgrade-postgres-to-18.sh production
# Type: UPGRADE PRODUCTION
```

### Step 3: Verify Upgrade

The script automatically verifies:

- ✅ Data counts match pre-upgrade
- ✅ PostgreSQL 18 is running
- ✅ Autocomplete indexes created
- ✅ Application health check passes

### Step 4: Test Functionality

```bash
# Test health endpoint
curl http://localhost:8080/health

# Test autocomplete (requires auth)
# Login first, then:
curl "http://localhost:8080/artists/search?q=test" \
  -H "Cookie: auth_token=YOUR_TOKEN"
```

## 🔙 Rollback Procedure

If something goes wrong, you can rollback:

```bash
# Find backup timestamp (in backups directory)
ls -la /opt/psychic-homily-<env>/backend/backups/

# Rollback using timestamp
./backend/scripts/rollback-postgres-upgrade.sh <environment> <timestamp>
# Example: ./backend/scripts/rollback-postgres-upgrade.sh stage 20250929_195433
```

## 📊 What the Upgrade Does

1. **Creates Backups**:

   - SQL dump: `/backups/pg17_to_pg18_TIMESTAMP.sql.gz`
   - Filesystem backup: `/backups/pg17_volume_TIMESTAMP.tar.gz`

2. **Upgrades Database**:

   - Removes old PostgreSQL 17 data volume
   - Starts fresh PostgreSQL 18 instance
   - Runs migrations (including new autocomplete indexes)
   - Restores all data from SQL dump

3. **Verifies Data**:
   - Checks row counts match
   - Verifies indexes created
   - Confirms application health

## 🆕 New Features in PostgreSQL 18

- Better performance for JSON operations
- Improved query optimization
- Enhanced security features
- Our new autocomplete indexes using `pg_trgm` extension

## 📝 Upgrade Timeline

| Environment    | When to Upgrade    | Estimated Downtime |
| -------------- | ------------------ | ------------------ |
| **Local Dev**  | ✅ Already done    | -                  |
| **Stage**      | Before production  | 5-10 minutes       |
| **Production** | After stage tested | 5-10 minutes       |

## 🔒 Safety Measures

1. **Multiple Backups**: Both SQL and filesystem backups created
2. **Pre-flight Checks**: Validates system before starting
3. **Data Verification**: Confirms data integrity after upgrade
4. **Rollback Script**: Can revert if issues occur
5. **Tested Process**: Already validated in local development

## 🐛 Troubleshooting

### Issue: "Cannot connect to database"

**Solution**: Wait 30 seconds for PostgreSQL to fully start, then retry

### Issue: "Disk space full"

**Solution**: Clear old backups or increase disk space before upgrading

### Issue: "Data counts don't match"

**Solution**: Immediately run rollback script and investigate

### Issue: "Application won't start after upgrade"

**Solution**: Check logs with `sudo journalctl -u psychic-homily-<env> -n 50`

## 📞 Emergency Contacts

If upgrade fails in production:

1. **Immediately run rollback script**
2. **Check application logs**
3. **Notify team**
4. **Document what went wrong**

## ✅ Post-Upgrade Checklist

- [ ] Verify data counts match pre-upgrade
- [ ] Test user login
- [ ] Test artist autocomplete search
- [ ] Test show creation
- [ ] Monitor logs for 24 hours
- [ ] Update documentation if needed
- [ ] Mark upgrade as complete in tracking

## 📁 Backup Retention

- **SQL Dumps**: Keep for 90 days
- **Filesystem Backups**: Keep for 30 days
- **Upgrade Logs**: Keep indefinitely

## 🎯 Success Criteria

Upgrade is considered successful when:

1. ✅ PostgreSQL 18 running
2. ✅ All data restored (counts match)
3. ✅ Application health check passes
4. ✅ Autocomplete search works
5. ✅ No errors in logs
6. ✅ Users can login and use features

---

**Created**: 2025-09-30  
**Last Updated**: 2025-09-30  
**Version**: 1.0
