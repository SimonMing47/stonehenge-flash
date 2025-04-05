package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main_test 是一个模拟的SMB-OnChain Mock程序，用于测试和演示目的
func main_test() {
	// 检查参数
	if len(os.Args) > 1 && os.Args[1] == "run" {
		logFile, err := os.OpenFile("smb-mock.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			defer logFile.Close()
			log.SetOutput(logFile)
		}

		configPath := "config.toml"
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}

		fmt.Printf("SMB-OnChain Mock 启动中，配置文件：%s\n", configPath)
		log.Printf("SMB-OnChain Mock 启动中，配置文件：%s\n", configPath)

		// 模拟启动信息
		fmt.Println("正在连接RPC节点...")
		log.Println("正在连接RPC节点...")
		time.Sleep(1 * time.Second)

		fmt.Println("读取池信息...")
		log.Println("读取池信息...")
		time.Sleep(1 * time.Second)

		fmt.Println("初始化监听器...")
		log.Println("初始化监听器...")
		time.Sleep(1 * time.Second)

		fmt.Println("SMB-OnChain Mock 已启动并正在运行")
		log.Println("SMB-OnChain Mock 已启动并正在运行")

		// 等待中断信号以优雅地关闭
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// 模拟持续运行，并定期输出日志
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sigCh:
				fmt.Println("接收到关闭信号，SMB-OnChain Mock 关闭中...")
				log.Println("接收到关闭信号，SMB-OnChain Mock 关闭中...")
				return
			case t := <-ticker.C:
				fmt.Printf("SMB-OnChain Mock 运行中... %s\n", t.Format("15:04:05"))
				log.Printf("SMB-OnChain Mock 运行中... %s\n", t.Format("15:04:05"))
				log.Printf("模拟检查套利机会...")
			}
		}
	} else {
		fmt.Println("用法: smb-onchain run [配置文件路径]")
		log.Println("用法: smb-onchain run [配置文件路径]")
		os.Exit(1)
	}
}
