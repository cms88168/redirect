package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// YAMLConfig 表示 config.yaml 中支持的配置项
// 仅支持最简单的 key: value 平铺格式，不依赖第三方 YAML 库
type YAMLConfig struct {
	Listen    string
	Proxy     string
	Remote    string
	KeepAlive bool
	TTL       int
	NoDelay   bool
}

// loadYAMLConfig 读取并解析 config.yaml 文件
func loadYAMLConfig(path string) (*YAMLConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &YAMLConfig{TTL: 30}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// 去掉行内注释（仅处理非引号中的 #，简化处理）
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")

		switch key {
		case "listen", "l":
			cfg.Listen = val
		case "proxy", "s":
			cfg.Proxy = val
		case "remote", "r":
			cfg.Remote = val
		case "keepalive":
			cfg.KeepAlive = parseYAMLBool(val)
		case "ttl":
			fmt.Sscanf(val, "%d", &cfg.TTL)
		case "nodelay":
			cfg.NoDelay = parseYAMLBool(val)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseYAMLBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "yes" || s == "1" || s == "on"
}

// ToConfig 将 YAMLConfig 转换为运行期 Config
func (yc *YAMLConfig) ToConfig() (*Config, error) {
	if yc.Listen == "" {
		return nil, fmt.Errorf("config.yaml 必须指定 listen")
	}
	if yc.Proxy == "" {
		return nil, fmt.Errorf("config.yaml 必须指定 proxy")
	}
	if yc.Remote == "" {
		return nil, fmt.Errorf("config.yaml 必须指定 remote")
	}

	config := &Config{
		ProxyAddr:    normalizeProxyAddr(yc.Proxy),
		KeepAlive:    yc.KeepAlive,
		KeepAliveSec: yc.TTL,
		NoDelay:      yc.NoDelay,
	}

	for _, addr := range strings.Split(yc.Listen, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if strings.HasPrefix(addr, "tcp://") {
			config.TCPListenAddr = strings.TrimPrefix(addr, "tcp://")
			config.EnableTCP = true
		} else if strings.HasPrefix(addr, "udp://") {
			config.UDPListenAddr = strings.TrimPrefix(addr, "udp://")
			config.EnableUDP = true
		} else {
			config.TCPListenAddr = addr
			config.EnableTCP = true
		}
	}

	for _, addr := range strings.Split(yc.Remote, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if strings.HasPrefix(addr, "tcp://") {
			config.TCPRemoteAddr = strings.TrimPrefix(addr, "tcp://")
		} else if strings.HasPrefix(addr, "udp://") {
			config.UDPRemoteAddr = strings.TrimPrefix(addr, "udp://")
		} else {
			config.TCPRemoteAddr = addr
			config.UDPRemoteAddr = addr
		}
	}

	if !config.EnableTCP && !config.EnableUDP {
		config.EnableTCP = true
	}

	return config, nil
}

// defaultConfigPath 返回可执行文件同目录下的 config.yaml 路径
func defaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml")
}
