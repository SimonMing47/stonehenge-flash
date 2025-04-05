package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"stonehenge-flash/agent"
)

func main() {
	// mevbot 配置文件路径
	tomlConfigPath := "config.toml"
	// flash agent 配置文件路径
	yamlConfigPath := "config.yaml"

	log.Println("启动MEV Bot监控代理...")

	// 创建代理实例
	a, err := agent.NewAgent(tomlConfigPath, yamlConfigPath)
	if err != nil {
		log.Fatalf("初始化代理失败: %v", err)
	}

	// 启动代理
	if err := a.Start(); err != nil {
		log.Fatalf("启动代理失败: %v", err)
	}

	// 捕获终止信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 等待终止信号
	<-sigCh
	log.Println("收到终止信号，开始关闭代理...")

	// 停止代理
	if err := a.Stop(); err != nil {
		log.Fatalf("关闭代理时出错: %v", err)
	}

	log.Println("代理已正常关闭")
}
