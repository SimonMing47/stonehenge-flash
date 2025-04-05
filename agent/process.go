package agent

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// ProcessManager 管理MEV Bot进程
type ProcessManager struct {
	name       string
	executable string
	args       []string // 新增: 命令行参数
	cmd        *exec.Cmd
	mutex      sync.RWMutex
	isRunning  bool
}

// NewProcessManager 创建新的进程管理器
func NewProcessManager(name, executable string, args ...string) *ProcessManager {
	return &ProcessManager{
		name:       name,
		executable: executable,
		args:       args, // 保存参数
		isRunning:  false,
	}
}

// Start 启动进程
func (p *ProcessManager) Start() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.isRunning {
		return nil
	}

	// 检查可执行文件是否存在
	if _, err := os.Stat(p.executable); os.IsNotExist(err) {
		return errors.New("找不到可执行文件: " + p.executable)
	}

	// 创建命令 - 使用参数
	p.cmd = exec.Command(p.executable, p.args...)

	// 设置标准输出和错误输出
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	// 启动进程
	if err := p.cmd.Start(); err != nil {
		return err
	}

	p.isRunning = true
	log.Printf("%s进程已启动, PID: %d, 命令: %s %v", p.name, p.cmd.Process.Pid, p.executable, p.args)

	// 监控进程
	go func() {
		err := p.cmd.Wait()

		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.isRunning = false

		if err != nil {
			log.Printf("%s进程已退出: %v", p.name, err)
		} else {
			log.Printf("%s进程已正常退出", p.name)
		}
	}()

	return nil
}

// Stop 停止进程
func (p *ProcessManager) Stop() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.isRunning || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	// 尝试优雅关闭
	log.Printf("正在优雅关闭%s进程 (PID: %d)...", p.name, p.cmd.Process.Pid)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Printf("发送SIGTERM信号失败: %v, 尝试强制终止", err)
		// 强制终止
		if err := p.cmd.Process.Kill(); err != nil {
			return err
		}
	}

	// 进程状态将在监控例程中更新
	return nil
}

// IsRunning 检查进程是否在运行
func (p *ProcessManager) IsRunning() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.isRunning
}
