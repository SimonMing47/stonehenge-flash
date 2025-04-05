package agent

import (
	"context"
	"log"
	"runtime"
	"sync"
	"time"
)

// Agent 监控和管理MEV Bot的代理程序
type Agent struct {
	mevConfigPath   string
	mevConfig       *Config
	agentConfig     *FlashAgentConfig
	proc            *ProcessManager
	ws              *WebSocketServer
	mu              sync.RWMutex
	isRunning       bool
	manuallyStopped bool // 新增: 标记是否为主动停止
	ctx             context.Context
	cancelFunc      context.CancelFunc
	statusChecks    chan struct{}
}

// NewAgent 创建一个新的代理实例
func NewAgent(mevConfigPath string, agentConfigPath string) (*Agent, error) {
	// 加载 MEV 配置文件
	mevConfig, err := LoadConfig(mevConfigPath)
	if err != nil {
		return nil, err
	}

	// 初始化 FlashAgent 配置文件
	agentConfig, err := LoadFlashAgentConfig(agentConfigPath)
	var logConfig *LogConfig
	if err != nil {
		log.Printf("加载FlashAgent配置文件失败，使用默认设置: %v", err)
		logConfig = GetDefaultLogConfig()
	} else {
		logConfig = &agentConfig.Logging
	}
	// 设置日志输出
	SetupLogger(logConfig)

	ctx, cancel := context.WithCancel(context.Background())

	agent := &Agent{
		mevConfig:    mevConfig,
		agentConfig:  agentConfig,
		isRunning:    false,
		ctx:          ctx,
		cancelFunc:   cancel,
		statusChecks: make(chan struct{}, 1),
	}

	// 创建进程管理器
	execName := "smb-onchain"
	if runtime.GOOS == "windows" {
		execName = "smb-onchain.exe"
	}

	agent.proc = NewProcessManager(
		"MEV Bot",     // 名称
		"./"+execName, // 可执行文件路径
		"run",
		"config.toml",
	)

	// 创建WebSocket服务器
	agent.ws = NewWebSocketServer(":8080", agent)

	return agent, nil
}

// Start 启动代理程序
func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isRunning {
		return nil
	}

	log.Println("启动MEV Bot代理...")

	// 启动WebSocket服务器
	if err := a.ws.Start(); err != nil {
		return err
	}

	// 启动MEV Bot进程
	if err := a.proc.Start(); err != nil {
		a.ws.Stop()
		return err
	}

	hotTokensTracker := NewHotTokensTracker(a.mevConfig, a.agentConfig, a)
	// 启动热点跟踪器
	go hotTokensTracker.StartTracking()

	// 启动状态监控
	go a.monitorStatus()

	a.isRunning = true
	log.Println("MEV Bot代理启动完成")

	return nil
}

// Stop 停止代理程序
func (a *Agent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isRunning {
		return nil
	}

	log.Println("停止MEV Bot代理...")

	// 标记为主动停止
	a.manuallyStopped = true

	// 停止监控
	a.cancelFunc()

	// 停止MEV Bot进程
	if err := a.proc.Stop(); err != nil {
		log.Printf("停止MEV Bot进程时出错: %v", err)
	}

	// 停止WebSocket服务器
	if err := a.ws.Stop(); err != nil {
		log.Printf("停止WebSocket服务器时出错: %v", err)
	}

	a.isRunning = false
	log.Println("MEV Bot代理已停止")

	return nil
}

// 新增方法: 手动停止MEV Bot但保持代理运行
func (a *Agent) StopMEVBot() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("手动停止MEV Bot...")

	// 标记为主动停止
	a.manuallyStopped = true

	// 停止MEV Bot
	if err := a.proc.Stop(); err != nil {
		return err
	}

	// 通知所有客户端
	a.ws.BroadcastMessage("MEV Bot已手动停止")

	return nil
}

// 新增方法: 手动启动MEV Bot
func (a *Agent) StartMEVBot() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("手动启动MEV Bot...")

	// 取消主动停止标记
	a.manuallyStopped = false

	// 启动MEV Bot
	if err := a.proc.Start(); err != nil {
		return err
	}

	// 通知所有客户端
	a.ws.BroadcastMessage("MEV Bot已手动启动")

	return nil
}

// RestartMEVBot 重启MEV Bot进程
func (a *Agent) RestartMEVBot() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("正在重启MEV Bot...")

	// 停止MEV Bot
	if err := a.proc.Stop(); err != nil {
		return err
	}

	// 标记为主动停止
	a.manuallyStopped = true

	// 启动MEV Bot
	if err := a.proc.Start(); err != nil {
		return err
	}
	// 标记为主动停止
	a.manuallyStopped = false

	// 通知所有客户端
	a.ws.BroadcastMessage("MEV Bot已重启")

	return nil
}

// UpdateConfig 更新配置文件并重启MEV Bot
func (a *Agent) UpdateConfig(updatedConfig *Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("更新MEV Bot配置...")

	// 保存配置文件
	if err := updatedConfig.SaveToFile(a.mevConfigPath); err != nil {
		return err
	}

	a.mevConfig = updatedConfig

	// 重启MEV Bot
	if err := a.proc.Stop(); err != nil {
		log.Printf("停止MEV Bot进程时出错: %v", err)
	}

	if err := a.proc.Start(); err != nil {
		log.Printf("启动MEV Bot进程时出错: %v", err)
		return err
	}

	// 通知所有客户端
	a.ws.BroadcastMessage("MEV Bot配置已更新并重启")

	return nil
}

// monitorStatus 监控MEV Bot的状态
func (a *Agent) monitorStatus() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			if !a.proc.IsRunning() && !a.manuallyStopped {
				// 只有在非主动停止的情况下才自动重启
				log.Println("检测到MEV Bot意外停止，尝试重启...")

				// 释放读锁，获取写锁以便修改状态
				a.mu.RUnlock()
				a.mu.Lock()

				if err := a.proc.Start(); err != nil {
					log.Printf("重启MEV Bot失败: %v", err)
					a.ws.BroadcastMessage("MEV Bot重启失败: " + err.Error())
				} else {
					log.Println("MEV Bot已成功重启")
					a.ws.BroadcastMessage("MEV Bot已自动重启")
				}

				a.mu.Unlock()
			} else {
				a.mu.RUnlock()
			}
		case <-a.statusChecks:
			// 手动检查状态
			status := "运行中"
			manualStatus := ""

			a.mu.RLock()
			if !a.proc.IsRunning() {
				status = "已停止"
				if a.manuallyStopped {
					manualStatus = " (手动停止)"
				} else {
					manualStatus = " (意外停止)"
				}
			}
			a.mu.RUnlock()

			a.ws.BroadcastMessage("MEV Bot状态: " + status + manualStatus)
		}
	}
}
