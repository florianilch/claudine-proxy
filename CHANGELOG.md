## Unreleased
### <!-- 1 -->Fixes
- Accept `budget_tokens` as string or number
- Bump Go version to fix resource exhaustion vulnerabilities

## 0.5.0 - 2025-11-22
### <!-- 0 -->Features
- Add static /v1/models endpoint for model discovery

### <!-- 1 -->Fixes
- Unify error handling in proxy and OpenAI adapter with OpenAI-compatible format.
- Return 413 status code when request size limit exceeded

## 0.4.0 - 2025-11-11
### <!-- 0 -->Features
- Add observability layer with structured JSON logs with trace correlation using W3C trace context
- Add basic readiness endpoints and shutdown delay for container orchestration and service management

### <!-- 1 -->Fixes
- Enforce maximum request body size to prevent memory exhaustion

## 0.3.0 - 2025-11-10
### <!-- 0 -->Features
- Add OpenAI compatibility handler for creating chat completions
- Add OpenAI compatibility layer for `/chat/completions` to Anthropic’s Message API endpoint

## 0.2.0 - 2025-11-08
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

