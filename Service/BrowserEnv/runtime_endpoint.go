package BrowserEnv

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"private_browser_client/Settings"
)

// publishedPortHostForService 返回当前 Go 服务访问 Docker 发布端口时应该使用的主机名。
//
// 设计来源：
//   - 用户在 Docker 化部署 Private_Browser_Client 后发现 web-vnc.html 连接失败；
//   - 根因是浏览器容器的 CDP/VNC 端口发布在 Docker 宿主机上，而服务容器里的
//     127.0.0.1 指向 Private_Browser_Client 自己，不是 Docker 宿主机；
//   - 同一个问题也会导致容器化服务执行 timezone CDP 探测时访问 127.0.0.1:cdpPort 失败。
//
// 职责边界：
// - 这里只解决“服务进程访问 Docker published port”的内部寻址；
// - 对外给浏览器/前端使用的 webVncUrl 仍然由 HTTP Host 生成，不能用这个内部地址替代；
// - 如果未来改成本地 Unix socket 管理 Docker，本函数会回退到 127.0.0.1，保持宿主机运行场景可用。
func publishedPortHostForService() string {
	if Settings.Conf == nil || Settings.Conf.DockerConfig == nil {
		return "127.0.0.1"
	}
	host := extractHostFromURL(Settings.Conf.DockerConfig.APIURL)
	if host == "" || strings.EqualFold(host, "localhost") {
		return "127.0.0.1"
	}
	return host
}

// publishedPortAddressForService 生成可被当前服务进程拨号的 host:port。
//
// 这里使用 net.JoinHostPort 是为了兼容 IPv6 地址，避免后续节点环境扩展时手动拼接出错。
func publishedPortAddressForService(port int) string {
	return net.JoinHostPort(publishedPortHostForService(), strconv.Itoa(port))
}

// publishedHTTPURLForService 生成当前服务访问发布端口的 HTTP URL。
//
// CDP 端口通过 Docker PortBinding 暴露为 HTTP 服务；服务容器内必须访问 Docker API
// 所在主机，而不是固定 127.0.0.1。
func publishedHTTPURLForService(port int, pathAndQuery string) string {
	if !strings.HasPrefix(pathAndQuery, "/") {
		pathAndQuery = "/" + pathAndQuery
	}
	return fmt.Sprintf("http://%s%s", publishedPortAddressForService(port), pathAndQuery)
}

// publishedCDPAddressForService 返回访问 Chrome CDP 时使用的 host:port。
//
// Chrome remote-debugging HTTP 会校验 Host 头：只能接受 IP 地址或 localhost。
// Docker 模式下服务容器通常通过 host.docker.internal 访问宿主机 published port，
// 但这个域名作为 Host 头会被 Chrome 拒绝并返回 500：
// "Host header is specified and is not an IP address or localhost."
// 因此 CDP HTTP / WebSocket URL 需要把可解析域名转换成 IP；VNC 是裸 TCP，不受这个限制。
func publishedCDPAddressForService(port int) string {
	host := publishedPortHostForService()
	if host == "" || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return net.JoinHostPort(host, strconv.Itoa(port))
	}
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if ip4 := ip.To4(); ip4 != nil {
				return net.JoinHostPort(ip4.String(), strconv.Itoa(port))
			}
		}
		for _, ip := range ips {
			return net.JoinHostPort(ip.String(), strconv.Itoa(port))
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// publishedCDPHTTPURLForService 生成当前服务访问 Chrome CDP 的 HTTP URL。
//
// 它和 publishedHTTPURLForService 的区别在于会尽量把 host.docker.internal 解析成 IP，
// 避免 Chrome CDP Host 头校验拒绝请求。
func publishedCDPHTTPURLForService(port int, pathAndQuery string) string {
	if !strings.HasPrefix(pathAndQuery, "/") {
		pathAndQuery = "/" + pathAndQuery
	}
	return fmt.Sprintf("http://%s%s", publishedCDPAddressForService(port), pathAndQuery)
}

// rewriteCDPWebSocketURLForService 把 Chrome 返回的 ws://127.0.0.1:端口 改写成服务可达地址。
//
// Chrome DevTools 返回的 webSocketDebuggerUrl 经常带 127.0.0.1。宿主机运行时这没问题，
// 但服务容器化时它会指回服务容器自身。这里只改写 host:port，不改 path/query，避免破坏
// DevTools target ID。
func rewriteCDPWebSocketURLForService(raw string, cdpPort int) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	}
	parsed.Host = publishedCDPAddressForService(cdpPort)
	return parsed.String()
}

func extractHostFromURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}
