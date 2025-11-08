# claudine – Use Your Claude Subscription Everywhere

Unlock your Claude Pro/Max subscription in any tool or library.

**Claudine** is a lightweight, session-free OAuth ambassador for Claude. It can be deployed as a local sidecar or as a shared service for development.

**Resilient Authentication:** Handles OAuth2 flow and token refresh, ensuring connections are long-lived and stable.

**Privacy by Design:** Designed as a pass-through proxy; never logs credentials or request/response bodies.

## Quick Start

**1. Install**

Via Homebrew (recommended for **macOS**):

```bash
brew install --cask florianilch/tap/claudine
```

For direct control, grab the [latest release](https://github.com/florianilch/claudine-proxy/releases/latest) for **Windows**, **Linux** or **macOS** and move it into your `PATH`.

<a href="https://github.com/florianilch/claudine-proxy/releases/latest"><img src="https://img.shields.io/github/v/release/florianilch/claudine-proxy?style=flat&logo=GitHub"></a>

**2. Authenticate**

```bash
claudine auth login
```

This kicks off a one-time login with your Claude account. Just follow the link, authorize the app using your Claude Pro/Max account and paste the code. Done.

**3. Run the Proxy**

```bash
claudine start
```
Claudine is now running at `http://localhost:4000`.

## Usage

Point any client or SDK at `http://localhost:4000`.

See [Anthropic's model docs](https://docs.anthropic.com/en/docs/about-claude/models) for available models.

### Native Anthropic API

Use this for tools that support Anthropic's API but not its OAuth flow.

```bash
curl http://localhost:4000/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: claudine" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 1024
  }'
```

**For SDK usage:**
- Point `base_url` to `http://localhost:4000`
- Set `api_key` to any value (proxy handles auth)
- See [Anthropic Python SDK](https://github.com/anthropics/anthropic-sdk-python) or [TypeScript SDK](https://github.com/anthropics/anthropic-sdk-typescript)

## Configuration

Claudine works out-of-the-box. Customize it with CLI flags, environment variables or a config file.

```bash
# Use a different port via a CLI flag (double-hyphen for nesting)
claudine start --server--port 9000

# Or use an environment variable (prefix with CLAUDINE_ and use __ for nesting)
export CLAUDINE_SERVER__PORT=9000
claudine start
```
Run `claudine --help` for all available options.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDINE_LOG_LEVEL` | Logging severity level | `info` |
| `CLAUDINE_LOG_FORMAT` | Log output format (`text` or `json`) | `text` |
| `CLAUDINE_SERVER__HOST` | Server bind address | `127.0.0.1` |
| `CLAUDINE_SERVER__PORT` | Server listen port | `4000` |

<details>
<summary><b>View all environment variables</b></summary>

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDINE_SHUTDOWN__TIMEOUT` | Graceful shutdown timeout | `10s` |
| `CLAUDINE_AUTH__STORAGE` | Token storage (`file`, `env`) | `file` |
| `CLAUDINE_AUTH__FILE` | Path for `file` storage | *Platform-dependent \** |
| `CLAUDINE_AUTH__ENV_KEY` | Env var for `env` storage |  |
| `CLAUDINE_AUTH__METHOD` | Auth method (`oauth` or `static`) | `oauth` |
| `CLAUDINE_UPSTREAM__BASE_URL` | Upstream API base URL | `https://api.anthropic.com/v1` |

\* Default locations for file storage:
- **Linux**: `~/.config/claudine-proxy/auth`
- **macOS**: `~/Library/Application Support/claudine-proxy/auth`
- **Windows**: `%AppData%\claudine-proxy\auth`

</details>

### Config File
For a persistent, declarative setup, you can use a `config.toml` file.

```toml
# config.toml
log_level = "info"
log_format = "json"

[server]
host = "127.0.0.1"
port = 8000

[auth]
storage = "file"
file = "~/.config/claudine_auth"
```

Then start the proxy with your config: `claudine start -c config.toml`

### Token Storage

Claudine securely handles your auth details.

| Storage   | Use Case                               |
|-----------|----------------------------------------|
| `file`    | **Default** Plain-text file. Good for systems without a native keychain. |
| `env`     | Reads from an env var. Escape hatch for ephemeral environments like CI/CD – won't auto-refresh. |

## Requirements

*   A **Claude Pro** or **Claude Max** subscription.
*   **To build from source:** Go 1.25+ with `GOEXPERIMENT=jsonv2` enabled.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
