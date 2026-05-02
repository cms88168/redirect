package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ProxyAddr     string
	TCPListenAddr string
	UDPListenAddr string
	TCPRemoteAddr string
	UDPRemoteAddr string
	EnableTCP     bool
	EnableUDP     bool
	KeepAlive     bool
	KeepAliveSec  int
	NoDelay       bool
}

func parseFlags() (*Config, error) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "选项:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "示例:")
		fmt.Fprintln(os.Stderr, "  监听TCP端口并转发:")
		fmt.Fprintln(os.Stderr, "    redirect.exe -l tcp://:8889 -s 127.0.0.1:1080 -r target.com:443")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  同时监听TCP和UDP:")
		fmt.Fprintln(os.Stderr, "    redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r target.com:443")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  绑定指定IP:")
		fmt.Fprintln(os.Stderr, "    redirect.exe -l tcp://127.0.0.1:8889 -s 127.0.0.1:1080 -r target.com:443")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  TCP和UDP使用不同目标地址:")
		fmt.Fprintln(os.Stderr, "    redirect.exe -l tcp://:8889,udp://:8889 -s 127.0.0.1:1080 -r tcp://tcp-target:443,udp://udp-target:53")
	}

	listen := flag.String("l", "", "本地监听地址，格式: [ip:]port、tcp://[ip:]port 或 udp://[ip:]port，多个用逗号分隔")
	proxy := flag.String("s", "", "SOCKS5代理地址，格式: host:port 或 socks5://host:port")
	remote := flag.String("r", "", "目标服务器地址，格式: host:port 或 tcp://host:port,udp://host:port")
	keepAlive := flag.Bool("keepalive", false, "启用TCP KeepAlive")
	keepAliveSec := flag.Int("ttl", 30, "TCP KeepAlive间隔(秒)")
	noDelay := flag.Bool("nodelay", false, "启用TCP_NODELAY")

	flag.Parse()

	if *listen == "" {
		return nil, fmt.Errorf("必须指定监听地址 (-l)")
	}
	if *proxy == "" {
		return nil, fmt.Errorf("必须指定SOCKS5代理地址 (-s)")
	}
	if *remote == "" {
		return nil, fmt.Errorf("必须指定目标服务器地址 (-r)")
	}

	config := &Config{
		ProxyAddr:    normalizeProxyAddr(*proxy),
		KeepAlive:    *keepAlive,
		KeepAliveSec: *keepAliveSec,
		NoDelay:      *noDelay,
	}

	listenAddrs := strings.Split(*listen, ",")
	for _, addr := range listenAddrs {
		addr = strings.TrimSpace(addr)
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

	remoteAddrs := strings.Split(*remote, ",")
	for _, addr := range remoteAddrs {
		addr = strings.TrimSpace(addr)
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

func normalizeProxyAddr(proxy string) string {
	if strings.HasPrefix(proxy, "socks5://") {
		u, err := url.Parse(proxy)
		if err == nil {
			return u.Host
		}
	}
	return proxy
}

func main() {
	// 无参数时：从 config.yaml 加载配置，并在 Windows 下最小化到系统托盘
	if len(os.Args) <= 1 {
		runNoArgsMode()
		return
	}

	config, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	runForwarders(config)
}

// runNoArgsMode 无命令行参数时的启动流程：读取 config.yaml + 托盘模式
func runNoArgsMode() {
	configPath := defaultConfigPath()
	yc, err := loadYAMLConfig(configPath)
	if err != nil {
		showFatal(fmt.Sprintf("无法加载配置文件:\n%s\n\n错误: %v\n\n请在可执行文件同目录创建 config.yaml，或使用命令行参数运行。", configPath, err))
		os.Exit(1)
	}
	config, err := yc.ToConfig()
	if err != nil {
		showFatal(fmt.Sprintf("配置文件错误: %v", err))
		os.Exit(1)
	}
	// Windows 下进入系统托盘，非 Windows 平台退化为前台运行
	runWithTrayIcon(config)
}

// runForwarders 以阻塞方式启动所有已启用的转发器
func runForwarders(config *Config) {
	log.Printf("配置: TCP监听=%s, UDP监听=%s, 代理=%s, TCP目标=%s, UDP目标=%s, TCP=%v, UDP=%v, KeepAlive=%v, TTL=%v, NoDelay=%v",
		config.TCPListenAddr, config.UDPListenAddr, config.ProxyAddr, config.TCPRemoteAddr, config.UDPRemoteAddr, config.EnableTCP, config.EnableUDP, config.KeepAlive, config.KeepAliveSec, config.NoDelay)

	var wg sync.WaitGroup

	if config.EnableTCP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startTCPForwarder(config); err != nil {
				log.Printf("TCP转发器错误: %v", err)
			}
		}()
	}

	if config.EnableUDP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startUDPForwarder(config); err != nil {
				log.Printf("UDP转发器错误: %v", err)
			}
		}()
	}

	wg.Wait()
}

func startTCPForwarder(config *Config) error {
	listener, err := net.Listen("tcp", config.TCPListenAddr)
	if err != nil {
		return fmt.Errorf("TCP监听失败 %s: %w", config.TCPListenAddr, err)
	}
	defer listener.Close()

	log.Printf("TCP监听开始: %s -> %s", config.TCPListenAddr, config.TCPRemoteAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("TCP接受连接失败: %v", err)
			continue
		}

		go handleTCPConnection(conn, config)
	}
}

func handleTCPConnection(localConn net.Conn, config *Config) {
	defer localConn.Close()

	log.Printf("TCP新连接: %s -> %s", localConn.RemoteAddr(), config.TCPRemoteAddr)

	proxyConn, err := connectViaSOCKS5(config.ProxyAddr, config.TCPRemoteAddr)
	if err != nil {
		log.Printf("SOCKS5连接失败: %v", err)
		return
	}
	defer proxyConn.Close()

	log.Printf("TCP转发建立: %s <-> %s (via %s)", localConn.RemoteAddr(), config.TCPRemoteAddr, config.ProxyAddr)

	applyTCPOptions(localConn, config)
	applyTCPOptions(proxyConn, config)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(proxyConn, localConn)
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(localConn, proxyConn)
	}()

	wg.Wait()
	log.Printf("TCP连接关闭: %s", localConn.RemoteAddr())
}

func applyTCPOptions(conn net.Conn, config *Config) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	if config.NoDelay {
		tcpConn.SetNoDelay(true)
	}
	if config.KeepAlive {
		tcpConn.SetKeepAlive(true)
		if config.KeepAliveSec > 0 {
			tcpConn.SetKeepAlivePeriod(time.Duration(config.KeepAliveSec) * time.Second)
		}
	}
}

func connectViaSOCKS5(proxyAddr, targetAddr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("连接SOCKS5代理失败: %w", err)
	}

	if err := socks5Handshake(conn); err != nil {
		conn.Close()
		return nil, err
	}

	if err := socks5Request(conn, targetAddr); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func socks5Handshake(conn net.Conn) error {
	req := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("SOCKS5握手请求失败: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5握手响应失败: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5版本不匹配: %d", resp[0])
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5认证失败: %d", resp[1])
	}

	return nil
}

func socks5Request(conn net.Conn, targetAddr string) error {
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("目标地址解析失败: %w", err)
	}

	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	ip := net.ParseIP(host)

	var req []byte
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append([]byte{0x05, 0x01, 0x00, 0x01}, ip4...)
		} else {
			req = append([]byte{0x05, 0x01, 0x00, 0x04}, ip.To16()...)
		}
	} else {
		req = append([]byte{0x05, 0x01, 0x00, 0x03}, byte(len(host)))
		req = append(req, []byte(host)...)
	}

	req = append(req, byte(port>>8), byte(port&0xff))

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("SOCKS5请求发送失败: %w", err)
	}

	resp := make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5响应读取失败: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5响应版本不匹配: %d", resp[0])
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5请求失败，状态码: %d", resp[1])
	}

	switch resp[3] {
	case 0x01:
		addr := make([]byte, 4+2)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return fmt.Errorf("SOCKS5读取IPv4地址失败: %w", err)
		}
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("SOCKS5读取域名长度失败: %w", err)
		}
		domainBuf := make([]byte, int(lenBuf[0])+2)
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return fmt.Errorf("SOCKS5读取域名失败: %w", err)
		}
	case 0x04:
		addr := make([]byte, 16+2)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return fmt.Errorf("SOCKS5读取IPv6地址失败: %w", err)
		}
	}

	return nil
}

func startUDPForwarder(config *Config) error {
	udpAddr, err := net.ResolveUDPAddr("udp", config.UDPListenAddr)
	if err != nil {
		return fmt.Errorf("UDP地址解析失败 %s: %w", config.UDPListenAddr, err)
	}

	localConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("UDP监听失败 %s: %w", config.UDPListenAddr, err)
	}
	defer localConn.Close()

	log.Printf("UDP监听开始: %s -> %s", config.UDPListenAddr, config.UDPRemoteAddr)

	type session struct {
		udpRelayConn *net.UDPConn
		tcpConn      net.Conn
		clientAddr   *net.UDPAddr
	}

	sessions := make(map[string]*session)
	var sessionsMu sync.Mutex

	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := localConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("UDP读取失败: %v", err)
			continue
		}

		clientKey := clientAddr.String()

		sessionsMu.Lock()
		s, exists := sessions[clientKey]
		if !exists {
			tcpConn, udpRelayAddr, err := establishUDPAssociation(config)
			if err != nil {
				log.Printf("UDP关联建立失败 %s: %v", clientAddr, err)
				sessionsMu.Unlock()
				continue
			}

			udpRelayConn, err := net.DialUDP("udp", nil, udpRelayAddr)
			if err != nil {
				log.Printf("连接SOCKS5 UDP中继失败: %v", err)
				tcpConn.Close()
				sessionsMu.Unlock()
				continue
			}

			s = &session{
				udpRelayConn: udpRelayConn,
				tcpConn:      tcpConn,
				clientAddr:   clientAddr,
			}
			sessions[clientKey] = s
			log.Printf("UDP会话建立: %s <-> %s (via %s)", clientAddr, config.UDPRemoteAddr, udpRelayAddr)

			go func(key string, sess *session) {
				defer func() {
					sess.udpRelayConn.Close()
					sess.tcpConn.Close()
					sessionsMu.Lock()
					delete(sessions, key)
					sessionsMu.Unlock()
					log.Printf("UDP会话关闭: %s", sess.clientAddr)
				}()

				relayBuf := make([]byte, 65535)
				for {
					n, _, err := sess.udpRelayConn.ReadFromUDP(relayBuf)
					if err != nil {
						if !isClosedError(err) {
							log.Printf("UDP从代理读取失败: %v", err)
						}
						return
					}

					data, err := unwrapSOCKS5UDP(relayBuf[:n])
					if err != nil {
						log.Printf("SOCKS5 UDP数据包解包失败: %v", err)
						continue
					}

					if _, err := localConn.WriteToUDP(data, sess.clientAddr); err != nil {
						if !isClosedError(err) {
							log.Printf("UDP转发到客户端失败: %v", err)
						}
						return
					}
				}
			}(clientKey, s)
		}
		sessionsMu.Unlock()

		packet := wrapSOCKS5UDP(buf[:n], config.UDPRemoteAddr)
		if _, err := s.udpRelayConn.Write(packet); err != nil {
			if !isClosedError(err) {
				log.Printf("UDP转发到代理失败: %v", err)
			}
			sessionsMu.Lock()
			delete(sessions, clientKey)
			sessionsMu.Unlock()
			s.udpRelayConn.Close()
			s.tcpConn.Close()
		}
	}
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "wsarecv") && strings.Contains(errStr, "closed")
}

func establishUDPAssociation(config *Config) (net.Conn, *net.UDPAddr, error) {
	proxyTCPAddr, err := net.ResolveTCPAddr("tcp", config.ProxyAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("解析SOCKS5代理地址失败: %w", err)
	}

	tcpConn, err := net.DialTCP("tcp", nil, proxyTCPAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接SOCKS5代理失败: %w", err)
	}

	if err := socks5Handshake(tcpConn); err != nil {
		tcpConn.Close()
		return nil, nil, err
	}

	req := []byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := tcpConn.Write(req); err != nil {
		tcpConn.Close()
		return nil, nil, fmt.Errorf("SOCKS5 UDP ASSOCIATE请求失败: %w", err)
	}

	resp := make([]byte, 4)
	if _, err := io.ReadFull(tcpConn, resp); err != nil {
		tcpConn.Close()
		return nil, nil, fmt.Errorf("SOCKS5 UDP ASSOCIATE响应失败: %w", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		tcpConn.Close()
		return nil, nil, fmt.Errorf("SOCKS5 UDP ASSOCIATE失败: %d %d", resp[0], resp[1])
	}

	var bindIP net.IP
	var bindPort int
	switch resp[3] {
	case 0x01:
		addr := make([]byte, 4+2)
		if _, err := io.ReadFull(tcpConn, addr); err != nil {
			tcpConn.Close()
			return nil, nil, err
		}
		bindIP = net.IP(addr[:4])
		bindPort = int(addr[4])<<8 | int(addr[5])
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(tcpConn, lenBuf); err != nil {
			tcpConn.Close()
			return nil, nil, err
		}
		domainBuf := make([]byte, int(lenBuf[0])+2)
		if _, err := io.ReadFull(tcpConn, domainBuf); err != nil {
			tcpConn.Close()
			return nil, nil, err
		}
		bindIP = net.ParseIP(string(domainBuf[:lenBuf[0]]))
		bindPort = int(domainBuf[lenBuf[0]])<<8 | int(domainBuf[lenBuf[0]+1])
	case 0x04:
		addr := make([]byte, 16+2)
		if _, err := io.ReadFull(tcpConn, addr); err != nil {
			tcpConn.Close()
			return nil, nil, err
		}
		bindIP = net.IP(addr[:16])
		bindPort = int(addr[16])<<8 | int(addr[17])
	}

	if bindIP == nil || bindIP.IsUnspecified() {
		bindIP = proxyTCPAddr.IP
	}

	udpAddr := &net.UDPAddr{
		IP:   bindIP,
		Port: bindPort,
	}

	return tcpConn, udpAddr, nil
}

func wrapSOCKS5UDP(data []byte, remoteAddr string) []byte {
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	ip := net.ParseIP(host)

	var header []byte
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			header = append([]byte{0x00, 0x00, 0x00, 0x01}, ip4...)
		} else {
			header = append([]byte{0x00, 0x00, 0x00, 0x04}, ip.To16()...)
		}
	} else {
		header = append([]byte{0x00, 0x00, 0x00, 0x03}, byte(len(host)))
		header = append(header, []byte(host)...)
	}

	header = append(header, byte(port>>8), byte(port&0xff))
	return append(header, data...)
}

func unwrapSOCKS5UDP(data []byte) ([]byte, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("SOCKS5 UDP数据包太短")
	}

	if data[0] != 0x00 || data[1] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 UDP数据包保留字段错误")
	}

	if data[2] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 UDP不支持分片: FRAG=%d", data[2])
	}

	var headerLen int
	switch data[3] {
	case 0x01:
		headerLen = 4 + 2 + 4
	case 0x03:
		if len(data) < 5 {
			return nil, fmt.Errorf("SOCKS5 UDP域名数据包太短")
		}
		domainLen := int(data[4])
		headerLen = 4 + 2 + 1 + domainLen
	case 0x04:
		headerLen = 4 + 2 + 16
	default:
		return nil, fmt.Errorf("SOCKS5 UDP不支持的地址类型: %d", data[3])
	}

	if len(data) < headerLen {
		return nil, fmt.Errorf("SOCKS5 UDP数据包头不完整")
	}

	return data[headerLen:], nil
}
