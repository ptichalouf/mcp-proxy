// The management API is gated by mcpProxy.options.authTokens when the operator
// sets them, and that is the configuration the docs recommend. The token is
// kept in localStorage so a reload of the page does not ask for it again; it is
// never sent anywhere except as the Authorization header of a same-origin
// /api/mcp request.
export const TOKEN_STORAGE_KEY = 'mcp-proxy-web.token';

const listeners = new Set<(token: string | null) => void>();

export function getToken(): string | null {
  if (typeof window === 'undefined') return null;
  const token = localStorage.getItem(TOKEN_STORAGE_KEY);
  return token ? token : null;
}

export function setToken(token: string | null): void {
  if (typeof window === 'undefined') return;
  if (token) {
    localStorage.setItem(TOKEN_STORAGE_KEY, token);
  } else {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
  }
  listeners.forEach((listener) => listener(token));
}

export function subscribeToToken(listener: (token: string | null) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
