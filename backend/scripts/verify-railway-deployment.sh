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

# Test 1: Health endpoint
echo "1. Testing health endpoint..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health")
if [ "$HTTP_CODE" = "200" ]; then
    echo "   ✓ Health check passed (HTTP $HTTP_CODE)"
else
    echo "   ✗ Health check failed (HTTP $HTTP_CODE)"
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
