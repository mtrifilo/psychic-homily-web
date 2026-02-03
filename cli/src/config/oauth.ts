import http from 'http';
import { URL } from 'url';
import open from 'open';
import { storeToken } from './auth.js';

export interface OAuthResult {
  success: boolean;
  token?: string;
  error?: string;
}

// Start a local server to receive the OAuth callback
export async function performOAuthLogin(
  apiUrl: string,
  environmentKey: string,
  provider: string = 'google'
): Promise<OAuthResult> {
  return new Promise((resolve) => {
    // Find an available port
    const server = http.createServer();

    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        resolve({ success: false, error: 'Failed to start local server' });
        return;
      }

      const port = address.port;
      const callbackUrl = `http://127.0.0.1:${port}/callback`;

      // Handle the callback
      server.on('request', (req, res) => {
        if (!req.url?.startsWith('/callback')) {
          res.writeHead(404);
          res.end('Not found');
          return;
        }

        const url = new URL(req.url, `http://127.0.0.1:${port}`);
        const token = url.searchParams.get('token');
        const error = url.searchParams.get('error');
        const expiresIn = url.searchParams.get('expires_in');

        // Send response to browser
        res.writeHead(200, { 'Content-Type': 'text/html' });

        if (token) {
          // Store the token
          const expiry = expiresIn ? parseInt(expiresIn, 10) : 86400; // Default 24h
          storeToken(environmentKey, token, expiry);

          res.end(`
            <!DOCTYPE html>
            <html>
              <head><title>Login Successful</title></head>
              <body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0;">
                <div style="text-align: center;">
                  <h1 style="color: #10b981;">✓ Login Successful</h1>
                  <p>You can close this window and return to the CLI.</p>
                </div>
              </body>
            </html>
          `);

          server.close();
          resolve({ success: true, token });
        } else {
          res.end(`
            <!DOCTYPE html>
            <html>
              <head><title>Login Failed</title></head>
              <body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0;">
                <div style="text-align: center;">
                  <h1 style="color: #ef4444;">✗ Login Failed</h1>
                  <p>${error || 'Unknown error'}</p>
                  <p>Please close this window and try again.</p>
                </div>
              </body>
            </html>
          `);

          server.close();
          resolve({ success: false, error: error || 'Login failed' });
        }
      });

      // Set a timeout
      const timeout = setTimeout(() => {
        server.close();
        resolve({ success: false, error: 'Login timed out' });
      }, 120000); // 2 minute timeout

      server.on('close', () => {
        clearTimeout(timeout);
      });

      // Build the OAuth URL with CLI callback
      const authUrl = `${apiUrl}/auth/login/${provider}?cli_callback=${encodeURIComponent(callbackUrl)}`;

      // Open the browser
      open(authUrl).catch(() => {
        server.close();
        resolve({ success: false, error: 'Failed to open browser' });
      });
    });

    server.on('error', (err) => {
      resolve({ success: false, error: `Server error: ${err.message}` });
    });
  });
}
