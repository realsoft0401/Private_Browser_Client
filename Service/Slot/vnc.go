package Slot

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	model "private_browser_client/Models/Slot"
	common "private_browser_client/Repository/Common"
	"private_browser_client/Settings"
)

var vncUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

// GetSlotVNCInfo 返回 slot 的 VNC/noVNC 连接信息。
//
// 设计来源：
// - WebVNC 已经收口为 `/web-vnc.html?slot=...`；
// - slot 本身是常驻资源对象，因此 VNC 连接对象也必须是 slot，而不是 package；
// - 这里只返回连接信息，不判断当前运行的是哪个 package。
func (s *Service) GetSlotVNCInfo(slotID string, httpBase string, wsBase string) (*model.VNCInfoResponse, error) {
	slot, err := s.GetSlotByID(slotID)
	if err != nil {
		return nil, err
	}
	if slot.VNCPort == nil || *slot.VNCPort <= 0 {
		return nil, common.ErrConflict
	}
	if slot.ContainerStatus == nil || strings.TrimSpace(*slot.ContainerStatus) != "running" {
		return nil, common.ErrConflict
	}

	return &model.VNCInfoResponse{
		SlotID:    slot.SlotID,
		VNCPort:   *slot.VNCPort,
		VNCURL:    publishedVNCURLForClient(httpBase, *slot.VNCPort),
		WSURL:     strings.TrimRight(wsBase, "/") + "/api/v1/edge/slots/" + url.PathEscape(slot.SlotID) + "/vnc/ws",
		WebVNCURL: strings.TrimRight(httpBase, "/") + "/web-vnc.html?slot=" + url.QueryEscape(slot.SlotID),
		CDPPort:   slot.CDPPort,
	}, nil
}

// GetSlotCDPInfo 返回 slot 视角的 CDP 连接信息。
//
// 设计来源：
// - 既然 WebVNC 已经切到 slot 视角，CDP 也必须跟着统一，不再让调用方继续猜 package 当前落在哪个 slot；
// - 当前只暴露 HTTP/version 入口，保持接口轻量，避免一开始就把复杂诊断和业务判断塞进来。
func (s *Service) GetSlotCDPInfo(slotID string, httpBase string) (*model.CDPInfoResponse, error) {
	slot, err := s.GetSlotByID(slotID)
	if err != nil {
		return nil, err
	}
	if slot.CDPPort == nil || *slot.CDPPort <= 0 {
		return nil, common.ErrConflict
	}
	if slot.ContainerStatus == nil || strings.TrimSpace(*slot.ContainerStatus) != "running" {
		return nil, common.ErrConflict
	}

	httpURL := publishedHTTPURLForClient(httpBase, *slot.CDPPort, "/")
	versionURL := publishedHTTPURLForClient(httpBase, *slot.CDPPort, "/json/version")
	wsBaseURL := publishedWSURLForClient(httpBase, *slot.CDPPort, "/devtools")
	return &model.CDPInfoResponse{
		SlotID:     slot.SlotID,
		CDPPort:    *slot.CDPPort,
		HTTPURL:    httpURL,
		VersionURL: versionURL,
		WSBaseURL:  &wsBaseURL,
	}, nil
}

// ProxySlotVNC 把 noVNC WebSocket 流量代理到 slot 的 VNC TCP 端口。
//
// 这里继续承担 websockify 角色：
// browser(noVNC) <-> WebSocket <-> Private_Browser_Client <-> Docker published port <-> x11vnc。
func (s *Service) ProxySlotVNC(writer http.ResponseWriter, request *http.Request, slotID string) error {
	slot, err := s.GetSlotByID(slotID)
	if err != nil {
		return err
	}
	if slot.VNCPort == nil || *slot.VNCPort <= 0 {
		return common.ErrConflict
	}
	if slot.ContainerStatus == nil || strings.TrimSpace(*slot.ContainerStatus) != "running" {
		return common.ErrConflict
	}

	targetAddr := publishedPortAddressForService(*slot.VNCPort)
	tcpConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("连接 slot vnc tcp 失败: %s: %w", targetAddr, err)
	}
	defer tcpConn.Close()

	wsConn, err := vncUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		return fmt.Errorf("websocket 升级失败: %w", err)
	}
	defer wsConn.Close()

	errCh := make(chan error, 2)
	go copyVNCToWebSocket(wsConn, tcpConn, errCh)
	go copyWebSocketToVNC(wsConn, tcpConn, errCh)
	<-errCh
	return nil
}

func copyVNCToWebSocket(wsConn *websocket.Conn, tcpConn net.Conn, errCh chan<- error) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := tcpConn.Read(buffer)
		if n > 0 {
			if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				errCh <- nil
				return
			}
			errCh <- err
			return
		}
	}
}

func copyWebSocketToVNC(wsConn *websocket.Conn, tcpConn net.Conn, errCh chan<- error) {
	for {
		messageType, reader, err := wsConn.NextReader()
		if err != nil {
			errCh <- err
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		if _, err = io.Copy(tcpConn, reader); err != nil {
			errCh <- err
			return
		}
	}
}

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

func publishedPortAddressForService(port int) string {
	return net.JoinHostPort(publishedPortHostForService(), strconv.Itoa(port))
}

func publishedVNCURLForClient(httpBase string, port int) string {
	host := extractHostFromURL(httpBase)
	if host == "" {
		host = publishedPortHostForService()
	}
	return "vnc://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func publishedHTTPURLForClient(httpBase string, port int, pathAndQuery string) string {
	host := extractHostFromURL(httpBase)
	if host == "" {
		host = publishedPortHostForService()
	}
	if !strings.HasPrefix(pathAndQuery, "/") {
		pathAndQuery = "/" + pathAndQuery
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + pathAndQuery
}

func publishedWSURLForClient(httpBase string, port int, pathAndQuery string) string {
	host := extractHostFromURL(httpBase)
	if host == "" {
		host = publishedPortHostForService()
	}
	if !strings.HasPrefix(pathAndQuery, "/") {
		pathAndQuery = "/" + pathAndQuery
	}
	return "ws://" + net.JoinHostPort(host, strconv.Itoa(port)) + pathAndQuery
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
