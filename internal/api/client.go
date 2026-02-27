package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// Client Xboard API 客户端
type Client struct {
	httpClient *http.Client
	baseURL    string // Xboard 面板地址
	token      string // 通信 token
	nodeID     int
	nodeType   string // 固定为 "anytls"
	userETag   string // 用户列表 ETag 缓存（含双引号，原样存储和发送）
	logger     *logrus.Logger
}

// NewClient 创建 API 客户端
func NewClient(baseURL, token string, nodeID int, nodeType string, logger *logrus.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		token:      token,
		nodeID:     nodeID,
		nodeType:   nodeType,
		logger:     logger,
	}
}

// doRequest 通用请求方法
// 自动拼接 URL: {baseURL}/api/v1/server/UniProxy/{path}?token=&node_id=&node_type=
// POST 请求自动设置 Content-Type: application/json
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	fullURL := fmt.Sprintf("%s/api/v1/server/UniProxy/%s", c.baseURL, path)

	params := url.Values{}
	params.Set("token", c.token)
	params.Set("node_id", strconv.Itoa(c.nodeID))
	params.Set("node_type", c.nodeType)
	fullURL += "?" + params.Encode()

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
