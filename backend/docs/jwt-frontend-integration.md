# JWT Frontend Integration Guide

This guide explains how to integrate with the JWT-based authentication system in your frontend application.

## **🔐 Authentication Flow**

### **1. OAuth Login Initiation**

```javascript
// Redirect user to OAuth provider
function initiateOAuthLogin(provider) {
  window.location.href = `/auth/login/${provider}`;
}

// Example usage
document.getElementById("google-login").addEventListener("click", () => {
  initiateOAuthLogin("google");
});
```

### **2. OAuth Callback Handling**

The OAuth callback will return a JSON response with the JWT token:

```javascript
// Handle OAuth callback
async function handleOAuthCallback() {
  try {
    const response = await fetch("/auth/callback");
    const data = await response.json();

    if (data.success) {
      // Store JWT token
      localStorage.setItem("auth_token", data.token);

      // Store user info
      localStorage.setItem("user", JSON.stringify(data.user));

      // Redirect to dashboard
      window.location.href = "/dashboard";
    } else {
      console.error("OAuth login failed");
    }
  } catch (error) {
    console.error("Error during OAuth callback:", error);
  }
}
```

### **3. API Calls with JWT**

```javascript
// Helper function to get stored token
function getAuthToken() {
  return localStorage.getItem("auth_token");
}

// Helper function to check if user is authenticated
function isAuthenticated() {
  const token = getAuthToken();
  return token !== null && token !== undefined;
}

// API call with JWT authentication
async function apiCall(endpoint, options = {}) {
  const token = getAuthToken();

  if (!token) {
    throw new Error("No authentication token found");
  }

  const defaultOptions = {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    ...options,
  };

  const response = await fetch(`/api/${endpoint}`, defaultOptions);

  if (response.status === 401) {
    // Token expired or invalid
    localStorage.removeItem("auth_token");
    localStorage.removeItem("user");
    window.location.href = "/login";
    return;
  }

  return response.json();
}

// Example API calls
async function getUserProfile() {
  return apiCall("auth/profile");
}

async function submitShow(showData) {
  return apiCall("show", {
    method: "POST",
    body: JSON.stringify(showData),
  });
}
```

### **4. Token Refresh**

```javascript
// Refresh JWT token
async function refreshToken() {
  try {
    const response = await apiCall("auth/refresh", {
      method: "POST",
    });

    if (response.body.success) {
      localStorage.setItem("auth_token", response.body.token);
      return response.body.token;
    }
  } catch (error) {
    console.error("Token refresh failed:", error);
    // Redirect to login
    window.location.href = "/login";
  }
}

// Auto-refresh token before expiry
function setupTokenRefresh() {
  // Check token every 5 minutes
  setInterval(async () => {
    const token = getAuthToken();
    if (token) {
      try {
        // Decode token to check expiry
        const payload = JSON.parse(atob(token.split(".")[1]));
        const expiry = payload.exp * 1000; // Convert to milliseconds
        const now = Date.now();

        // Refresh if token expires in next 30 minutes
        if (expiry - now < 30 * 60 * 1000) {
          await refreshToken();
        }
      } catch (error) {
        console.error("Error checking token expiry:", error);
      }
    }
  }, 5 * 60 * 1000); // 5 minutes
}
```

### **5. Logout**

```javascript
// Logout user
async function logout() {
  try {
    await apiCall("auth/logout", {
      method: "POST",
    });
  } catch (error) {
    console.error("Logout error:", error);
  } finally {
    // Clear local storage
    localStorage.removeItem("auth_token");
    localStorage.removeItem("user");

    // Redirect to login
    window.location.href = "/login";
  }
}
```

## **📱 Mobile App Integration (React Native)**

```javascript
import AsyncStorage from "@react-native-async-storage/async-storage";

// Store JWT token
async function storeToken(token) {
  await AsyncStorage.setItem("auth_token", token);
}

// Get JWT token
async function getToken() {
  return await AsyncStorage.getItem("auth_token");
}

// API call for mobile
async function mobileApiCall(endpoint, options = {}) {
  const token = await getToken();

  if (!token) {
    throw new Error("No authentication token found");
  }

  const response = await fetch(`/api/${endpoint}`, {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    ...options,
  });

  if (response.status === 401) {
    // Clear token and redirect to login
    await AsyncStorage.removeItem("auth_token");
    // Navigate to login screen
    return;
  }

  return response.json();
}

// OAuth login for mobile
async function mobileOAuthLogin(provider) {
  // Use WebView or deep linking for OAuth
  // After successful OAuth, store the token
  const token = await getOAuthToken(provider);
  await storeToken(token);
}
```

## **🔒 Security Best Practices**

### **1. Token Storage**

- **Web**: Use `localStorage` for convenience or `httpOnly` cookies for security
- **Mobile**: Use secure storage (Keychain for iOS, Keystore for Android)

### **2. Token Expiry**

- Set reasonable expiry times (24 hours for regular tokens)
- Implement automatic refresh before expiry
- Clear tokens on logout

### **3. Error Handling**

- Handle 401 responses by redirecting to login
- Implement retry logic for network errors
- Show user-friendly error messages

### **4. CORS Configuration**

```javascript
// Frontend should include credentials
fetch("/api/endpoint", {
  credentials: "include",
  headers: {
    Authorization: `Bearer ${token}`,
  },
});
```

## **🧪 Testing**

### **1. Test Authentication Flow**

```javascript
// Test OAuth login
async function testOAuthFlow() {
  console.log("Testing OAuth flow...");

  // 1. Initiate login
  initiateOAuthLogin("google");

  // 2. After callback, test API call
  const profile = await getUserProfile();
  console.log("User profile:", profile);

  // 3. Test show submission
  const showData = {
    artists: [{ name: "Test Artist" }],
    venue: "Test Venue",
    date: "2025-01-15",
  };

  const result = await submitShow(showData);
  console.log("Show submission result:", result);
}
```

### **2. Test Token Refresh**

```javascript
// Test token refresh
async function testTokenRefresh() {
  console.log("Testing token refresh...");

  const newToken = await refreshToken();
  console.log("New token:", newToken);
}
```

## **🚀 Environment Variables**

Add these to your frontend environment:

```bash
# Frontend environment variables
REACT_APP_API_URL=http://localhost:8080
REACT_APP_OAUTH_REDIRECT_URL=http://localhost:3000/auth/callback
```

## **📊 Migration from Sessions**

If migrating from session-based authentication:

1. **Update API calls** to include `Authorization` header
2. **Remove cookie handling** code
3. **Implement token storage** in localStorage
4. **Add token refresh logic**
5. **Update error handling** for 401 responses

The JWT implementation provides better scalability and mobile support while maintaining security! 🎯
