package api

import (
	"fmt"
	"net/http"
	"time"
)

// 重试间隔：1s, 2s, 4s
var retryDelays = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
}

// doWithRetry 带指数退避重试的请求执行
// 仅对 5xx 和网络错误重试，4xx 不重试
func doWithRetry(fn func() (*http.Response, error)) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		resp, err := fn()
		if err != nil {
			// 网络错误，重试
			lastErr = err
			if attempt < len(retryDelays) {
				time.Sleep(retryDelays[attempt])
				continue
			}
			return nil, fmt.Errorf("请求失败（已重试 %d 次）: %w", len(retryDelays), lastErr)
		}

		// 4xx 不重试
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return resp, nil
		}

		// 5xx 重试
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("服务端错误: %d", resp.StatusCode)
			if attempt < len(retryDelays) {
				time.Sleep(retryDelays[attempt])
				continue
			}
			return nil, fmt.Errorf("请求失败（已重试 %d 次）: %w", len(retryDelays), lastErr)
		}

		// 2xx/3xx 成功
		return resp, nil
	}

	return nil, fmt.Errorf("请求失败（已重试 %d 次）: %w", len(retryDelays), lastErr)
}
