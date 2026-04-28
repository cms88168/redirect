[中文](README_zh.md) | English

# SOCKS5 Port Forwarding Tool

A SOCKS5 proxy port forwarding tool supporting both TCP and UDP.

## Features

- **TCP Forwarding**: Forward local TCP port traffic to target servers through a SOCKS5 proxy
- **UDP Forwarding**: Support SOCKS5 UDP ASSOCIATE for UDP traffic forwarding
- **Dual Protocol Listening**: Simultaneously listen on the same port for TCP and UDP
- **Bind Specific IP**: Support binding to specific IP addresses (IPv4/IPv6)
- **TCP Optimization**: Support KeepAlive and TCP_NODELAY
- **Protocol Differentiation**: TCP and UDP can specify different target addresses

## Usage

### Basic Usage

```powershell
redirect.exe -l <listen_address> -s <SOCKS5_proxy> -r <target_address>
```

### Parameters

| Parameter | Required | Description | Example |
|-----------|----------|-------------|---------|
| `-l` | Yes | Local listen address | `tcp://:8889` |
| `-s` | Yes | SOCKS5 proxy address | `127.0.0.1:1080` |
| `-r` | Yes | Target server address | `target.com:443` |
| `-keepalive` | No | Enable TCP KeepAlive (default false) | `-keepalive` |
| `-ttl` | No | TCP KeepAlive interval in seconds (default 30) | `-ttl=10` |
| `-nodelay` | No | Enable TCP_NODELAY (default false) | `-nodelay` |
| `-h` | No | Show help information | `-h` |

### Address Formats

**Listen Address (`-l`)**:
- `tcp://[ip:]port` - Listen on TCP port
- `udp://[ip:]port` - Listen on UDP port
- `[ip:]port` - Default to TCP listening
- Multiple addresses separated by commas

**Proxy Address (`-s`)**:
- `host:port`
- `socks5://host:port`

**Target Address (`-r`)**:
- `host:port` - Same target for TCP and UDP
- `tcp://host:port,udp://host:port` - Specify targets separately

## Examples

### 1. Basic TCP Forwarding

Forward local port 8889 TCP traffic through SOCKS5 proxy to target.com:443:

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 2. Listen on TCP and UDP Simultaneously

Listen on both TCP and UDP port 8889:

```powershell
redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 3. Bind to Specific IP

Only listen on 127.0.0.1:8889:

```powershell
redirect.exe -l tcp://127.0.0.1:8889 -s 127.0.0.1:1080 -r target.com:443
```

Bind to IPv6 address:

```powershell
redirect.exe -l tcp://[::1]:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 4. Different Targets for TCP and UDP

Forward TCP to tcp-target:443 and UDP to udp-target:53:

```powershell
redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r tcp://tcp-target:443,udp://udp-target:53
```

### 5. Adjust TCP Parameters

Enable KeepAlive and TCP_NODELAY:

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443 -keepalive -nodelay
```

Set KeepAlive interval to 10 seconds:

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443 -keepalive -ttl=10
```

## Build

```powershell
# Windows
go build -o redirect.exe main.go

# Linux/Mac
go build -o redirect main.go
```

## Notes

1. TCP and UDP use different protocol stacks, so they can bind to the same IP and port simultaneously
2. UDP forwarding depends on SOCKS5 proxy's UDP ASSOCIATE support
3. Target addresses support IPv4, IPv6, and domain names
