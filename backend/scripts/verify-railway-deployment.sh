#!/bin/bash
# Railway Deployment Verification Script
#
# Usage:
#   ./verify-railway-deployment.sh stage    # Verify stage deployment
#   ./verify-railway-deployment.sh prod     # Verify production deployment

set -e

ENV=${1:-stage}

if [ "$ENV" = "stage" ]; then
    BASE_URL="https://stage.api.psychichomily.com"
elif [ "$ENV" = "prod" ] || [ "$ENV" = "production" ]; then
    BASE_URL="https://api.psychichomily.com"
else
    echo "Usage: $0 [stage|prod]"
    exit 1
fi

echo "=== Verifying Railway Deployment: $ENV ==="
echo "Base URL: $BASE_URL"
echo ""

# Test 1: Readiness endpoint
#
# Deliberately /health/ready and NOT /health. /health is a liveness probe: it
# returns 200 whenever the process is serving, even with the database
# unreachable, because Railway restarts the service when it fails. Verifying a
# deployment against it therefore proves only "a process is listening" — which
# is precisely how a total database outage passed verification before.
#
# Nothing restarts on /health/ready's result, so it is safe to gate on here.
echo "1. Testing readiness endpoint..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health/ready")
if [ "$HTTP_CODE" = "200" ]; then
    echo "   ✓ Readiness check passed (HTTP $HTTP_CODE)"
else
    echo "   ✗ Readiness check failed (HTTP $HTTP_CODE) — process may be up but a critical dependency is unreachable"
    exit 1
fi

# Test 2: Venues API (public endpoint)
echo "2. Testing venues API..."
RESPONSE=$(curl -s "$BASE_URL/api/venues")
if echo "$RESPONSE" | grep -q '\['; then
    echo "   ✓ Venues API responded with JSON array"
else
    echo "   ✗ Venues API failed or returned unexpected response"
    echo "   Response: $RESPONSE"
fi

# Test 3: SSL Certificate
echo "3. Checking SSL certificate..."
SSL_EXPIRY=$(echo | openssl s_client -servername ${BASE_URL#https://} -connect ${BASE_URL#https://}:443 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
if [ -n "$SSL_EXPIRY" ]; then
    echo "   ✓ SSL certificate valid, expires: $SSL_EXPIRY"
else
    echo "   ⚠ Could not verify SSL certificate"
fi

# Test 4: Google OAuth redirect
echo "4. Testing Google OAuth endpoint..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/auth/google")
if [ "$HTTP_CODE" = "302" ] || [ "$HTTP_CODE" = "307" ]; then
    echo "   ✓ OAuth redirect working (HTTP $HTTP_CODE)"
else
    echo "   ⚠ OAuth endpoint returned HTTP $HTTP_CODE (expected 302/307)"
fi

# Test 5: CORS headers
echo "5. Testing CORS headers..."
if [ "$ENV" = "stage" ]; then
    ORIGIN="https://stage.psychichomily.com"
else
    ORIGIN="https://psychichomily.com"
fi
CORS_HEADER=$(curl -s -I -H "Origin: $ORIGIN" "$BASE_URL/health" | grep -i "access-control-allow-origin" || echo "")
if echo "$CORS_HEADER" | grep -q "$ORIGIN"; then
    echo "   ✓ CORS headers present for $ORIGIN"
else
    echo "   ⚠ CORS headers may not be configured correctly"
    echo "   Header: $CORS_HEADER"
fi

echo ""
echo "=== Verification Complete ==="
echo ""
echo "Manual tests still needed:"
echo "- [ ] Google OAuth full login flow"
echo "- [ ] User registration with email verification"
echo "- [ ] Passkey registration (if enabled)"
echo "- [ ] Magic link login"
echo "- [ ] Frontend can connect and fetch data"
