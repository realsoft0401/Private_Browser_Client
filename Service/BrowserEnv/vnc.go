package BrowserEnv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
)

var vncUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

// GetBrowserEnvVNCInfo 返回环境包的浏览器 VNC 连接信息。
//
// 设计来源：
// - 用户在 Mac 上使用原生 VNC 时遇到密码弹窗；
// - 容器内 x11vnc 当前使用 -nopw，问题主要来自原生客户端交互，不应给 VNC 额外加固定密码；
// - 因此推荐前端打开 webVncUrl，让 noVNC 通过边缘服务 WebSocket 代理连接 VNC。
//
// 职责边界：
// - 只返回连接信息，不启动容器；
// - 只从 SQLite 索引读取 vncPort，不允许前端传目标端口；
// - 后续加鉴权时应在 HTTP 层或中心服务端做会话校验，不要把裸 VNC 端口暴露给公网。
func (s *Service) GetBrowserEnvVNCInfo(envID string, httpBase string, wsBase string) (*model.BrowserEnvVNCInfoResponse, error) {
	index, err := getRuntimeIndex(envID)
	if err != nil {
		return nil, err
	}
	if index.VNCPort <= 0 {
		return nil, conflictError("环境包未分配 VNC 端口")
	}
	if index.Status == model.BrowserEnvStatusDeleted {
		return nil, conflictError("环境包已删除，不能打开 VNC")
	}
	if index.Status != model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包未运行，不能返回 VNC 连接信息")
	}
	envID = index.EnvID
	return &model.BrowserEnvVNCInfoResponse{
		EnvID:     envID,
		VNCPort:   index.VNCPort,
		VNCURL:    fmt.Sprintf("vnc://127.0.0.1:%d", index.VNCPort),
		WSURL:     strings.TrimRight(wsBase, "/") + "/api/v1/edge/browser-envs/" + envID + "/vnc/ws",
		WebVNCURL: strings.TrimRight(httpBase, "/") + "/web-vnc.html?envId=" + envID,
	}, nil
}

// ProxyBrowserEnvVNC 代理 noVNC WebSocket 到浏览器容器的 VNC TCP 端口。
//
// 它在 Go 服务里承担 websockify 的角色：
// browser(noVNC) <-> WebSocket <-> Private_Browser_Client <-> Docker published port <-> x11vnc。
//
// 维护原则：
//   - 目标端口只能来自 browser_envs，不能通过 query 参数传入；
//   - 这个代理只做字节转发，不理解 VNC 协议，不保存画面或剪贴板内容；
//   - 不能把目标地址写死为 127.0.0.1：服务容器化运行时 127.0.0.1 是服务容器自己，
//     必须根据 Docker API 地址选择 host.docker.internal 或真实宿主机地址；
//   - 后续商业化必须加鉴权/临时 token，避免任何人拿 envId 就能进入远程桌面。
func (s *Service) ProxyBrowserEnvVNC(writer http.ResponseWriter, request *http.Request, envID string) error {
	index, err := getRuntimeIndex(envID)
	if err != nil {
		return err
	}
	if index.VNCPort <= 0 {
		return conflictError("环境包未分配 VNC 端口")
	}
	if index.Status != model.BrowserEnvStatusRunning {
		return conflictError("环境包未运行，不能连接 VNC")
	}

	targetAddr := publishedPortAddressForService(index.VNCPort)
	tcpConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
	if err != nil {
		return remoteError(fmt.Sprintf("连接 VNC TCP 失败: %s: %v", targetAddr, err))
	}
	defer tcpConn.Close()

	wsConn, err := vncUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		return remoteError(fmt.Sprintf("WebSocket 升级失败: %v", err))
	}
	defer wsConn.Close()

	errCh := make(chan error, 2)
	go copyVNCToWebSocket(wsConn, tcpConn, errCh)
	go copyWebSocketToVNC(wsConn, tcpConn, errCh)
	<-errCh
	return nil
}

func getRuntimeIndex(envID string) (*model.BrowserEnvIndex, error) {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, invalidError("envId 不能为空")
	}
	index, err := browserEnvDao.NewRuntimeModelHandler().GetBrowserEnvIndexByID(context.Background(), envID)
	if err != nil {
		if errors.Is(err, browserEnvDao.ErrBrowserEnvNotFound) {
			return nil, notFoundError("环境包不存在")
		}
		return nil, internalError(err.Error())
	}
	return index, nil
}

// copyVNCToWebSocket 把 x11vnc 的 TCP 字节转发给浏览器 noVNC。
//
// WebSocket 使用 binary frame；不做文本转换，否则 VNC 握手和像素数据会损坏。
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

// copyWebSocketToVNC 把浏览器 noVNC 发来的二进制帧写入 VNC TCP 连接。
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
