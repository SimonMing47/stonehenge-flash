package agent

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

// FlashAgentConfig 表示整个代理配置
type FlashAgentConfig struct {
	Logging LogConfig     `yaml:"logging"` // 日志配置
	Ave     AveConfig     `yaml:"ave"`     // Ave服务配置
	Wechat  WechatConfig  `yaml:"wechat"`  // 微信配置
	SolScan SolScanConfig `yaml:"solscan"` // Solscan配置
}

// LogConfig 表示日志配置
type LogConfig struct {
	OutputPath string `yaml:"output_path"` // 日志文件路径
	MaxSize    int    `yaml:"max_size"`    // 单个日志文件最大大小，MB
	MaxBackups int    `yaml:"max_backups"` // 最大保留旧日志文件数
	MaxAge     int    `yaml:"max_age"`     // 保留旧日志文件的最大天数
	Compress   bool   `yaml:"compress"`    // 是否压缩旧日志文件
	LocalTime  bool   `yaml:"local_time"`  // 使用本地时间而非UTC时间
}

// AveConfig 表示Ave服务配置
type AveConfig struct {
	Token string `yaml:"token"` // Ave服务认证令牌
}

// WechatConfig 表示微信配置
type WechatConfig struct {
	VerifyToken string `yaml:"verify_token"` // 微信连接校验token
}

type SolScanConfig struct {
	SolAuth string `yaml:"sol_auth"` // Solscan的身份验证信息
	Token   string `yaml:"token"`    // Solscan的身份验证令牌
	Cookie  string `yaml:"cookie"`   // Solscan的Cookie
	Origin  string `yaml:"origin"`   // Solscan的API地址
	Referer string `yaml:"referer"`  // Solscan的Referer头
}

// LoadFlashAgentConfig 从YAML文件加载代理配置
func LoadFlashAgentConfig(path string) (*FlashAgentConfig, error) {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("配置文件 %s 不存在", path)
	}

	// 读取文件内容
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析YAML
	var config FlashAgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析YAML配置失败: %w", err)
	}

	// 设置默认值（如果必要的话）
	if config.Logging.OutputPath == "" {
		config.Logging.OutputPath = "flash.log"
	}
	if config.Logging.MaxSize <= 0 {
		config.Logging.MaxSize = 100 // 默认100MB
	}
	if config.Logging.MaxBackups <= 0 {
		config.Logging.MaxBackups = 10
	}
	if config.Logging.MaxAge <= 0 {
		config.Logging.MaxAge = 30
	}

	return &config, nil
}

// GetDefaultLogConfig 返回默认日志配置
func GetDefaultLogConfig() *LogConfig {
	return &LogConfig{
		OutputPath: "flash.log",
		MaxSize:    100,  // 100MB
		MaxBackups: 5,    // 保留5个旧文件
		MaxAge:     30,   // 30天
		Compress:   true, // 压缩旧文件
		LocalTime:  true, // 使用本地时间
	}
}

// SetupLogger 配置日志输出系统
// 根据配置设置日志输出到文件和控制台
func SetupLogger(config *LogConfig) error {
	// 创建日志目录（如果不存在）
	logDir := filepath.Dir(config.OutputPath)
	if logDir != "" && logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}
	}

	// 设置日志轮换
	logRotator := &lumberjack.Logger{
		Filename:   config.OutputPath,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
		LocalTime:  config.LocalTime,
	}

	// 同时输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, logRotator)
	log.SetOutput(multiWriter)

	// 设置日志格式，包含日期、时间和文件信息
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("日志已配置为输出到: %s", config.OutputPath)

	return nil
}

// SaveFlashAgentConfig 将配置保存回YAML文件
func SaveFlashAgentConfig(config *FlashAgentConfig, path string) error {
	// 将配置转换为YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 写入文件
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
