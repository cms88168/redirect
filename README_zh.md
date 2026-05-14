中文 | [English](README.md)

# SOCKS5 端口转发工具

一个支持 TCP 和 UDP 的 SOCKS5 代理端口转发工具。

## 功能特性

- **TCP 转发**：将本地 TCP 端口流量通过 SOCKS5 代理转发到目标服务器
- **UDP 转发**：支持 SOCKS5 UDP ASSOCIATE，实现 UDP 流量转发
- **双协议同时监听**：可同时监听 TCP 和 UDP 相同端口
- **绑定指定 IP**：支持绑定特定 IP 地址（IPv4/IPv6）
- **TCP 优化**：支持 KeepAlive 和 TCP_NODELAY
- **协议区分**：TCP 和 UDP 可分别指定不同的目标地址
- **配置文件支持**：无参数运行时自动读取 `config.yaml`
- **Windows 托盘运行**：无参数运行时在 Windows 下最小化到右下角系统托盘，而不是任务栏

## 使用方法

### 方式一：双击运行（推荐 Windows 用户）

在 `redirect.exe` 同目录下放置一个 `config.yaml` 文件，然后直接双击 `redirect.exe`：

- 程序会自动隐藏控制台窗口，在系统托盘（屏幕右下角）显示图标
- 右键点击托盘图标可选择“退出”
- 日志会写入 exe 同目录下的 `redirect.log`

`config.yaml` 示例（可参考仓库中的 `config.yaml.example`）：

```yaml
listen: tcp://:8889,udp://:8889
proxy: 127.0.0.1:1080
remote: target.com:443
keepalive: false
ttl: 30
nodelay: false
```

支持的键与命令行参数一一对应：`listen`(`-l`)、`proxy`(`-s`)、`remote`(`-r`)、`keepalive`、`ttl`、`nodelay`。

### 方式二：命令行参数运行

```powershell
redirect.exe -l <监听地址> -s <SOCKS5代理> -r <目标地址>
```

使用任意命令行参数时，程序按普通控制台模式运行，不会进入托盘。

### 参数说明

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| `-l` | 是 | 本地监听地址 | `tcp://:8889` |
| `-s` | 是 | SOCKS5 代理地址 | `127.0.0.1:1080` |
| `-r` | 是 | 目标服务器地址 | `target.com:443` |
| `-keepalive` | 否 | 启用 TCP KeepAlive（默认 false） | `-keepalive` |
| `-ttl` | 否 | TCP KeepAlive 间隔，单位秒（默认 30） | `-ttl=10` |
| `-nodelay` | 否 | 启用 TCP_NODELAY（默认 false） | `-nodelay` |
| `-h` | 否 | 显示帮助信息 | `-h` |

### 地址格式

**监听地址 (`-l`)**：
- `tcp://[ip:]port` - 监听 TCP 端口
- `udp://[ip:]port` - 监听 UDP 端口
- `[ip:]port` - 默认监听 TCP
- 多个地址用逗号分隔

**代理地址 (`-s`)**：
- `host:port`
- `socks5://host:port`

**目标地址 (`-r`)**：
- `host:port` - TCP 和 UDP 使用相同目标
- `tcp://host:port,udp://host:port` - 分别指定目标

## 使用示例

### 1. 基本 TCP 转发

将本地 8889 端口的 TCP 流量通过 SOCKS5 代理转发到 target.com:443：

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 2. 同时监听 TCP 和 UDP

同时监听 TCP 和 UDP 的 8889 端口：

```powershell
redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 3. 绑定指定 IP

只监听 127.0.0.1:8889：

```powershell
redirect.exe -l tcp://127.0.0.1:8889 -s 127.0.0.1:1080 -r target.com:443
```

绑定 IPv6 地址：

```powershell
redirect.exe -l tcp://[::1]:8889 -s 127.0.0.1:1080 -r target.com:443
```

### 4. TCP 和 UDP 使用不同目标

TCP 转发到 tcp-target:443，UDP 转发到 udp-target:53：

```powershell
redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r tcp://tcp-target:443,udp://udp-target:53
```

### 5. 调整 TCP 参数

启用 KeepAlive 和 TCP_NODELAY：

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443 -keepalive -nodelay
```

设置 KeepAlive 间隔为 10 秒：

```powershell
redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443 -keepalive -ttl=10
```

## 编译

```powershell
# Windows（推荐：GUI 子系统，双击运行时不产生任何控制台/终端窗口）
go build -ldflags "-H=windowsgui -s -w" -o redirect.exe .

# Windows（调试用：保留控制台子系统）
go build -o redirect.exe .

# Linux/Mac
go build -o redirect .
```

> ⚠️ 在已安装 Windows Terminal 的系统上，必须使用 `-H=windowsgui` 构建，否则 Windows Terminal 的宿主窗口无法被隐藏，会残留在任务栏。

## 注意事项

1. TCP 和 UDP 使用不同的协议栈，可以同时绑定相同的 IP 和端口
2. UDP 转发依赖 SOCKS5 代理的 UDP ASSOCIATE 支持
3. 目标地址支持 IPv4、IPv6 和域名
4. 无参数启动（双击运行）时，`config.yaml` 必须与 `redirect.exe` 位于同一目录
5. 托盘模式下日志输出到 exe 同目录下的 `redirect.log`，可在该文件中排查问题
6. 系统托盘功能仅在 Windows 下生效，Linux/macOS 下无参数模式会以前台方式运行
