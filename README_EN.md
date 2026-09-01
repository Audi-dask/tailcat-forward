English | [简体中文](./README.md)

# tailcat-forward

Map TCP ports exposed by a [tailcat](https://github.com/tailscale/tailcat) server to ordinary local TCP listening ports.

Let tools that only understand `host:port` — browsers, database clients (MySQL/Redis/PostgreSQL), IDEs, scripts, etc. — reach the remote tailcat server without any SOCKS or stdio support.

Built on the `github.com/tailscale/tailcat` library; tailcat itself is not modified.

## Installation

### Download a Release binary

Grab the archive for your platform from the [Releases](https://github.com/Audi-dask/tailcat-forward/releases) page (linux / darwin / windows × amd64 / arm64).

### Build from source

```bash
go install github.com/Audi-dask/tailcat-forward@latest
```

## Quick Start

### 1. Expose a port on the server

On the remote server, use tailcat to expose a local port:

```bash
tailcat serve 8080
# 🐈 Server listening with new address: tcXXXXXXXXXXXXXXXX...
```

Copy the `tc...` token from the output.

### 2. Map the port locally

```bash
tailcat-forward tcXXXXXXXXXXXXXXXX... 18080:8080
# forwarding 127.0.0.1:18080 -> remote localhost:8080
```

### 3. Reach it with ordinary tools

```bash
curl http://127.0.0.1:18080/
```

Or just open `http://127.0.0.1:18080/` in your browser.

## Usage

```text
tailcat-forward [--bind=<addr>] <addrblob> <[local:]remote> [<[local:]remote> ...]
```

| Example | Effect |
|---|---|
| `tailcat-forward <token> 8080` | local `127.0.0.1:8080` → remote `8080` |
| `tailcat-forward <token> 18080:8080` | local `127.0.0.1:18080` → remote `8080` |
| `tailcat-forward --bind=0.0.0.0 <token> 18080:8080` | other devices on your LAN can reach it too |
| `tailcat-forward <token> 3306:3306 6379:6379` | map MySQL + Redis in one go |

The `addrblob` may also be a DNS name carrying a `tailcat=` TXT record.

## Examples

Map a remote Jenkins dashboard to localhost:

```bash
tailcat-forward <token> 18080:8080
```

Then open `http://127.0.0.1:18080/` in your browser.

Map remote MySQL and Redis at once:

```bash
tailcat-forward <token> 3306:3306 6379:6379
mysql -h 127.0.0.1 -P 3306 -u root -p
redis-cli -h 127.0.0.1 -p 6379
```

## Notes

- Every connection reuses the same tailcat client, avoiding repeated handshakes and NAT traversal.
- `ProxyConns` copies bidirectionally and propagates the TCP half-close, so protocols that only respond after the request body ends work fine.
- `Ctrl-C` shuts down gracefully.
