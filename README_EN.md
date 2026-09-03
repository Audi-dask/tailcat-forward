English | [简体中文](./README.md)

# tailcat-forward

> **Preparing for archival: this is a feature-validation repository based on the official `tailcat`.**

When client-side TCP forwarding was not yet available in the official [tailscale/tailcat](https://github.com/tailscale/tailcat), I implemented and validated the capability on top of its codebase. It lets browsers, database clients, IDEs, scripts, and other tools that only understand ordinary `host:port` connections reach remote Tailcat services.

The implementation was submitted upstream:

- ✅ [PR #62](https://github.com/tailscale/tailcat/pull/62) — added `tailcat forward`; merged
- 🚧 [PR #75](https://github.com/tailscale/tailcat/pull/75) — forwarding to Exit Node targets; in progress

This repository is kept as a record of the validation and implementation process. For the current upstream implementation, see:

👉 [tailscale/tailcat](https://github.com/tailscale/tailcat)

> This repository is no longer maintained as an independent project. The installation and usage sections below are kept to reproduce the original validation setup.

## Historical validation: Installation

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
