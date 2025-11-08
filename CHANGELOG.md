## Unreleased
### <!-- 0 -->Features
- Make keyring the default storage
- Add keyring as an additional storage type

## 0.1.0 - 2025-11-06
### <!-- 0 -->Features
- Add static token source to allow managing token rotation outside of app
- Add OAuth authorization code flow with PKCE
- Replace hard-coded config and allow customization using CLI flags, environment variables or a config file
- Add cli layer to entrypoint
- Add impersonation transport to messages endpoint
- Implement persistent token source for Claude
- Add observability layer with basic slog handler
- Add logging and recovery middleware
- Register reverse proxy handler for `messages` endpoint
- Setup basic app and service structure

### <!-- 1 -->Fixes
- Bump Go version to fix multiple DoS and resource exhaustion vulnerabilities
- Fixes host header injection which leads to open redirect

