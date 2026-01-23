# Email Verification Runbook

This document outlines the steps needed to configure and test the email verification flow in Stage (and Production).

## Overview

The email verification system requires users to verify their email address before they can submit shows (unless they're an admin or signed up via OAuth). The flow uses:

- **Resend** for transactional email delivery
- **JWT tokens** with 24-hour expiry for verification links
- **Frontend verification page** at `/verify-email`

---

## Prerequisites

Before deploying, ensure you have:

- [ ] A Resend account (https://resend.com)
- [ ] A verified sending domain in Resend
- [ ] Access to your deployment environment variables

---

## Step 1: Resend Account Setup

1. **Create a Resend account** at https://resend.com if you don't have one

2. **Add and verify your domain**:
   - Go to Resend Dashboard → Domains
   - Add your domain (e.g., `psychichomily.com`)
   - Add the required DNS records (SPF, DKIM, DMARC)
   - Wait for verification (usually a few minutes)

3. **Generate an API key**:
   - Go to Resend Dashboard → API Keys
   - Create a new API key with "Sending access"
   - Copy the key (you'll only see it once)

---

## Step 2: Environment Variables

Add these environment variables to your **backend** deployment:

| Variable | Description | Example |
|----------|-------------|---------|
| `RESEND_API_KEY` | Your Resend API key | `re_xxxxxxxx` |
| `FROM_EMAIL` | Sender email address (must match verified domain) | `noreply@psychichomily.com` |
| `FRONTEND_URL` | Base URL for verification links | See below |

### FRONTEND_URL by Environment

| Environment | Value |
|-------------|-------|
| Local | `http://localhost:3000` |
| Stage | `https://stage.psychichomily.com` |
| Production | `https://psychichomily.com` |

### Example .env additions

```bash
# Email Configuration
RESEND_API_KEY=re_your_api_key_here
FROM_EMAIL=noreply@psychichomily.com
FRONTEND_URL=https://stage.psychichomily.com
```

---

## Step 3: Verify Deployment

After deploying with the new environment variables:

1. **Check backend logs** for any email configuration errors on startup

2. **Verify the config is loaded** by checking that the email service initializes without errors

---

## Step 4: Test the Flow

### Test Case 1: Send Verification Email

1. Create a new account with email/password (not OAuth)
2. Log in and go to Settings
3. Find the "Email Verification" section
4. Click "Send Verification Email"
5. **Expected**: Success message appears, email is received

### Test Case 2: Verify Email via Link

1. Open the verification email
2. Click the "Verify Email" button
3. **Expected**: Redirects to `/verify-email?token=...` and shows success message

### Test Case 3: Unverified User Blocked from Submitting Shows

1. Log in as an unverified user
2. Try to submit a new show
3. **Expected**: 403 error with message about email verification required

### Test Case 4: Verified User Can Submit Shows

1. After verifying email, try to submit a show
2. **Expected**: Show submission works normally

### Test Case 5: Expired Token

1. Wait 24+ hours (or manually test with an expired token)
2. Click old verification link
3. **Expected**: Error message with option to request new email

### Test Case 6: OAuth Users Auto-Verified

1. Sign up/log in via OAuth (Google, etc.)
2. Check Settings → Email Verification
3. **Expected**: Shows as already verified

---

## Troubleshooting

### Email not received

- Check Resend dashboard for delivery status
- Verify the `FROM_EMAIL` domain is verified in Resend
- Check spam/junk folders
- Ensure `RESEND_API_KEY` is correct

### "Invalid token" error on verification

- Token may have expired (24-hour limit)
- User should request a new verification email
- Check that `JWT_SECRET_KEY` is the same across deployments

### Verification link goes to wrong URL

- Check `FRONTEND_URL` environment variable
- Ensure it matches the actual frontend deployment URL
- No trailing slash should be included

### 403 error but user is verified

- User may need to refresh their session
- Check that `email_verified` is `true` in the database
- Verify the profile refetch is working after confirmation

---

## File Locations

For reference, here are the key files in the implementation:

### Backend

| File | Purpose |
|------|---------|
| `backend/internal/services/email.go` | Email sending via Resend |
| `backend/internal/services/jwt.go` | Verification token creation/validation |
| `backend/internal/api/handlers/auth.go` | API endpoints (lines 470-650) |
| `backend/internal/config/config.go` | Email configuration struct |

### Frontend

| File | Purpose |
|------|---------|
| `frontend/app/verify-email/page.tsx` | Verification confirmation page |
| `frontend/components/SettingsPanel.tsx` | Send verification button UI |
| `frontend/lib/hooks/useAuth.ts` | `useSendVerificationEmail` and `useConfirmVerification` hooks |

---

## Future Improvements (Optional)

These are not required but could enhance the system:

- [ ] Rate limiting on verification email sends
- [ ] Auto-send verification email on registration
- [ ] Token revocation when new email is requested
- [ ] Verification reminder emails after N days
- [ ] Add email config to `backend/.env.example`
