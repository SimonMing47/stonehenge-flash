package agent

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketServer 提供WebSocket服务
type WebSocketServer struct {
	addr      string
	upgrader  websocket.Upgrader
	clients   map[*websocket.Conn]bool
	broadcast chan string
	agent     *Agent
	server    *http.Server
	mu        sync.Mutex
}

// Command 表示WebSocket命令
type Command struct {
	Type    string          `json:"type"`
	Action  string          `json:"action"`
	Section string          `json:"section,omitempty"`
	Key     string          `json:"key,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}

// NewWebSocketServer 创建新的WebSocket服务器
func NewWebSocketServer(addr string, agent *Agent) *WebSocketServer {
	return &WebSocketServer{
		addr: addr,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源的连接
			},
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan string, 100),
		agent:     agent,
	}
}

// Start 启动WebSocket服务器
func (ws *WebSocketServer) Start() error {
	mux := http.NewServeMux()

	// WebSocket端点
	mux.HandleFunc("/ws", ws.handleConnections)

	// 启动广播器
	go ws.broadcastMessages()

	// 创建HTTP服务器
	ws.server = &http.Server{
		Addr:    ws.addr,
		Handler: mux,
	}

	// 启动HTTP服务器
	go func() {
		log.Printf("WebSocket服务器开始监听: %s", ws.addr)
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket服务器错误: %v", err)
		}
	}()

	return nil
}

// Stop 停止WebSocket服务器
func (ws *WebSocketServer) Stop() error {
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}

// handleConnections 处理新的WebSocket连接
func (ws *WebSocketServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	// 验证token
	token := r.URL.Query().Get("token")
	expectedToken := ws.agent.agentConfig.Wechat.VerifyToken

	if expectedToken != "" && token != expectedToken {
		log.Printf("WebSocket连接验证失败: 无效的token")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	// 注册新客户端
	ws.mu.Lock()
	ws.clients[conn] = true
	ws.mu.Unlock()

	// 发送欢迎消息
	conn.WriteJSON(map[string]string{
		"type":    "system",
		"message": "已连接到MEV Bot代理",
	})

	// 处理客户端消息
	go ws.handleMessages(conn)
}

// handleMessages 处理来自客户端的消息
func (ws *WebSocketServer) handleMessages(conn *websocket.Conn) {
	defer func() {
		// 客户端断开连接时清理
		ws.mu.Lock()
		delete(ws.clients, conn)
		ws.mu.Unlock()
		conn.Close()
	}()

	for {
		// 读取消息
		var cmd Command
		err := conn.ReadJSON(&cmd)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket错误: %v", err)
			}
			break
		}

		// 处理命令
		ws.handleCommand(conn, &cmd)
	}
}

// handleCommand 处理客户端发送的命令
func (ws *WebSocketServer) handleCommand(conn *websocket.Conn, cmd *Command) {
	log.Printf("收到命令: %+v", cmd)

	response := map[string]interface{}{
		"type":   "response",
		"action": cmd.Action,
	}

	var err error

	switch cmd.Type {
	case "config":
		switch cmd.Action {
		case "get":
			// 获取当前配置
			response["data"] = ws.agent.mevConfig
		case "update":
			// 更新配置
			err = ws.handleConfigUpdate(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "配置已更新"
			}
		case "updateSection":
			// 更新配置节
			err = ws.handleSectionUpdate(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "配置节已更新"
			}
		case "addMint":
			// 添加铸币配置
			err = ws.handleAddMint(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "铸币配置已添加"
			}
		case "removeMint":
			// 删除铸币配置
			err = ws.handleRemoveMint(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "铸币配置已删除"
			}
		}
	case "bot":
		switch cmd.Action {
		case "status":
			// 请求状态检查
			ws.agent.statusChecks <- struct{}{}
			response["message"] = "状态检查已触发"
		case "restart":
			// 重启MEV Bot
			err = ws.agent.RestartMEVBot()
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "MEV Bot已重启"
			}
		case "updateRPC":
			// 更新RPC地址
			err = ws.handleUpdateRPC(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "RPC地址已更新"
			}
		case "toggleFeature":
			// 切换功能开关
			err = ws.handleToggleFeature(cmd)
			if err != nil {
				response["error"] = err.Error()
			} else {
				response["message"] = "功能状态已更新"
			}
		}
	default:
		response["error"] = "未知命令类型"
	}

	// 发送响应
	conn.WriteJSON(response)
}

// broadcastMessages 广播消息到所有连接的客户端
func (ws *WebSocketServer) broadcastMessages() {
	for {
		msg := <-ws.broadcast

		ws.mu.Lock()
		for client := range ws.clients {
			err := client.WriteJSON(map[string]string{
				"type":    "notification",
				"message": msg,
			})
			if err != nil {
				log.Printf("广播消息失败: %v", err)
				client.Close()
				delete(ws.clients, client)
			}
		}
		ws.mu.Unlock()
	}
}

// BroadcastMessage 发送消息到所有客户端
func (ws *WebSocketServer) BroadcastMessage(message string) {
	ws.broadcast <- message
}

// 配置更新处理程序
func (ws *WebSocketServer) handleConfigUpdate(cmd *Command) error {
	var updatedConfig Config
	if err := json.Unmarshal(cmd.Value, &updatedConfig); err != nil {
		return err
	}

	return ws.agent.UpdateConfig(&updatedConfig)
}

// 配置节更新处理程序
func (ws *WebSocketServer) handleSectionUpdate(cmd *Command) error {
	// 复制当前配置
	updatedConfig := ws.agent.mevConfig.Copy()

	// 根据节和键更新值
	var value interface{}
	if err := json.Unmarshal(cmd.Value, &value); err != nil {
		return err
	}

	if err := updatedConfig.UpdateSection(cmd.Section, cmd.Key, value); err != nil {
		return err
	}

	// 保存更新后的配置
	return ws.agent.UpdateConfig(updatedConfig)
}

// 添加铸币配置处理程序
func (ws *WebSocketServer) handleAddMint(cmd *Command) error {
	var mintConfig MintConfig
	if err := json.Unmarshal(cmd.Value, &mintConfig); err != nil {
		return err
	}

	// 复制当前配置
	updatedConfig := ws.agent.mevConfig.Copy()

	// 添加铸币配置
	updatedConfig.Routing.MintConfigList = append(updatedConfig.Routing.MintConfigList, mintConfig)

	// 保存更新后的配置
	return ws.agent.UpdateConfig(updatedConfig)
}

// 删除铸币配置处理程序
func (ws *WebSocketServer) handleRemoveMint(cmd *Command) error {
	var mintAddress string
	if err := json.Unmarshal(cmd.Value, &mintAddress); err != nil {
		return err
	}

	// 复制当前配置
	updatedConfig := ws.agent.mevConfig.Copy()

	// 查找并删除铸币配置
	newMintList := make([]MintConfig, 0)
	for _, mint := range updatedConfig.Routing.MintConfigList {
		if mint.Mint != mintAddress {
			newMintList = append(newMintList, mint)
		}
	}

	updatedConfig.Routing.MintConfigList = newMintList

	// 保存更新后的配置
	return ws.agent.UpdateConfig(updatedConfig)
}

// 更新RPC地址处理程序
func (ws *WebSocketServer) handleUpdateRPC(cmd *Command) error {
	var rpcConfig struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(cmd.Value, &rpcConfig); err != nil {
		return err
	}

	// 复制当前配置
	updatedConfig := ws.agent.mevConfig.Copy()

	// 更新RPC URL
	updatedConfig.RPC.URL = rpcConfig.URL

	// 保存更新后的配置
	return ws.agent.UpdateConfig(updatedConfig)
}

// 切换功能开关处理程序
func (ws *WebSocketServer) handleToggleFeature(cmd *Command) error {
	var featureConfig struct {
		Feature string `json:"feature"`
		Enabled bool   `json:"enabled"`
	}

	if err := json.Unmarshal(cmd.Value, &featureConfig); err != nil {
		return err
	}

	// 复制当前配置
	updatedConfig := ws.agent.mevConfig.Copy()

	// 更新功能开关
	switch featureConfig.Feature {
	case "spam":
		updatedConfig.Spam.Enabled = featureConfig.Enabled
	case "jito":
		updatedConfig.Jito.Enabled = featureConfig.Enabled
	case "kamino_flashloan":
		updatedConfig.KaminoFlashloan.Enabled = featureConfig.Enabled
	case "merge_mints":
		updatedConfig.Bot.MergeMints = featureConfig.Enabled
	}

	// 保存更新后的配置
	return ws.agent.UpdateConfig(updatedConfig)
}
