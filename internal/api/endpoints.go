package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"anytls/proxy/padding"
)

// NodeConfig 从 API 获取的节点配置
type NodeConfig struct {
	ServerPort    int      `json:"server_port"`
	ServerName    string   `json:"server_name"`
	PaddingScheme []string `json:"padding_scheme"`
	BaseConfig    struct {
		PushInterval int `json:"push_interval"`
		PullInterval int `json:"pull_interval"`
	} `json:"base_config"`
}

// User 用户信息
// speed_limit/device_limit 可以为 null，使用 *int 指针类型
type User struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	SpeedLimit  *int   `json:"speed_limit"`  // Mbps, nil 或 0=不限
	DeviceLimit *int   `json:"device_limit"` // nil 或 0=不限
}

// PaddingSchemeToBytes 将 Xboard 返回的 padding_scheme 数组转换为换行分隔格式
// Xboard: ["stop=8", "0=30-30", ...] -> anytls-go: "stop=8\n0=30-30\n..."
func PaddingSchemeToBytes(scheme []string) []byte {
	return []byte(strings.Join(scheme, "\n"))
}

// FetchConfig 获取节点配置
// 获取 padding_scheme 后自动调用 padding.UpdatePaddingScheme() 更新全局 padding
func (c *Client) FetchConfig() (*NodeConfig, error) {
	resp, err := doWithRetry(func() (*http.Response, error) {
		return c.doRequest(http.MethodGet, "config", nil)
	})
	if err != nil {
		return nil, fmt.Errorf("获取节点配置失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var cfg NodeConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("解析节点配置失败: %w", err)
	}

	// 转换 padding_scheme 并更新全局 padding
	if len(cfg.PaddingScheme) > 0 {
		rawScheme := PaddingSchemeToBytes(cfg.PaddingScheme)
		if padding.UpdatePaddingScheme(rawScheme) {
			c.logger.Info("padding scheme 已更新")
		} else {
			c.logger.Warn("padding scheme 更新失败，格式可能不正确")
		}
	}

	return &cfg, nil
}

// FetchUsers 获取用户列表（支持 ETag）
// 返回 nil, nil 表示 304 未修改
// ETag 处理：Xboard 返回 ETag: "abc123"（含双引号），原样存储和发送
func (c *Client) FetchUsers() ([]User, error) {
	resp, err := doWithRetry(func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/api/v1/server/UniProxy/user?token=%s&node_id=%d&node_type=%s",
				c.baseURL, c.token, c.nodeID, c.nodeType), nil)
		if err != nil {
			return nil, err
		}
		// 原样发送 ETag（含双引号）
		if c.userETag != "" {
			req.Header.Set("If-None-Match", c.userETag)
		}
		return c.httpClient.Do(req)
	})
	if err != nil {
		return nil, fmt.Errorf("获取用户列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		c.logger.Debug("用户列表未变化 (304)")
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Users []User `json:"users"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析用户列表失败: %w", err)
	}

	// 原样存储 ETag（含双引号，如 "abc123"）
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.userETag = etag
	}

	c.logger.WithField("count", len(result.Users)).Info("用户列表已更新")
	return result.Users, nil
}
