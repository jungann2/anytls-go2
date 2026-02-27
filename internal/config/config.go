package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 服务端配置结构
type Config struct {
	Listen     string    `yaml:"listen"`    // 监听地址，如 "0.0.0.0:8443"
	APIHost    string    `yaml:"api_host"`  // Xboard API 地址
	APIToken   string    `yaml:"api_token"` // 通信 token
	NodeID     int       `yaml:"node_id"`   // 节点 ID
	NodeType   string    `yaml:"node_type"` // 节点类型，默认 "anytls"
	TLS        TLSConfig `yaml:"tls"`
	Log        LogConfig `yaml:"log"`
	Fallback   string    `yaml:"fallback"`   // fallback 目标地址
	Standalone bool      `yaml:"standalone"` // 独立运行模式（不依赖 Xboard）
	Password   string    `yaml:"password"`   // 独立模式密码
}

// TLSConfig TLS 证书配置
type TLSConfig struct {
	CertFile string `yaml:"cert_file"` // 证书文件路径
	KeyFile  string `yaml:"key_file"`  // 私钥文件路径
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string `yaml:"level"`     // 日志级别: debug, info, warn, error
	FilePath string `yaml:"file_path"` // 日志文件路径
}

// LoadConfig 从 YAML 文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{
		Listen:   "0.0.0.0:8443",
		NodeType: "anytls",
		Log: LogConfig{
			Level: "info",
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 验证配置完整性
func (c *Config) Validate() error {
	if c.Standalone {
		// 独立模式只需要密码
		if c.Password == "" {
			return fmt.Errorf("配置错误: standalone 模式下 password 不能为空")
		}
	} else {
		// Xboard 模式需要 API 配置
		if c.APIHost == "" {
			return fmt.Errorf("配置错误: api_host 不能为空")
		}
		if c.APIToken == "" {
			return fmt.Errorf("配置错误: api_token 不能为空")
		}
		if c.NodeID <= 0 {
			return fmt.Errorf("配置错误: node_id 必须大于 0")
		}
	}
	if c.Listen == "" {
		c.Listen = "0.0.0.0:8443"
	}
	if c.NodeType == "" {
		c.NodeType = "anytls"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	return nil
}
