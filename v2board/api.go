// Package v2board 提供与 V2board 面板的 API 交互功能，
// 包括节点配置拉取、用户列表获取和流量数据上报。
package v2board

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// nodeType 是向 V2board 面板标识的节点类型
	nodeType = "anytls"

	// defaultHTTPTimeout 是 API 请求的默认超时时间
	defaultHTTPTimeout = 10 * time.Second
)

// Client 是 V2board HTTP API 客户端
type Client struct {
	httpClient *http.Client
	apiHost    string
	apiKey     string
	nodeID     uint
}

// NewClient 创建一个新的 V2board API 客户端
func NewClient(apiHost, apiKey string, nodeID uint) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		apiHost:    apiHost,
		apiKey:     apiKey,
		nodeID:     nodeID,
	}
}

// buildURL 构造带鉴权参数的 API URL
func (c *Client) buildURL(path string) string {
	params := url.Values{
		"token":     {c.apiKey},
		"node_id":   {strconv.Itoa(int(c.nodeID))},
		"node_type": {nodeType},
	}
	return c.apiHost + path + "?" + params.Encode()
}

// ---- 节点配置 ----

// BaseConfig 是节点基础配置中的间隔参数
type BaseConfig struct {
	// PushInterval 是流量上报周期（秒）
	PushInterval int `json:"push_interval"`
	// PullInterval 是用户列表拉取周期（秒）
	PullInterval int `json:"pull_interval"`
}

// NodeInfo 是 V2board 返回的节点配置信息
type NodeInfo struct {
	// ServerPort 是服务器监听端口
	ServerPort uint `json:"server_port"`
	// BaseConfig 包含上报/拉取周期配置
	BaseConfig BaseConfig `json:"base_config"`
}

// GetNodeInfo 从 V2board 面板拉取当前节点的配置信息
func (c *Client) GetNodeInfo() (*NodeInfo, error) {
	apiURL := c.buildURL("/api/v1/server/UniProxy/config")
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求节点配置失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("节点配置 API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取节点配置响应失败: %w", err)
	}

	var nodeInfo NodeInfo
	if err = json.Unmarshal(body, &nodeInfo); err != nil {
		return nil, fmt.Errorf("解析节点配置 JSON 失败: %w, 原始内容: %s", err, string(body))
	}

	return &nodeInfo, nil
}

// ---- 用户列表 ----

// User 代表一个 V2board 中的代理用户
type User struct {
	// ID 是用户的数字 ID，用于流量上报
	ID int `json:"id"`
	// UUID 是用户的 UUID，用作客户端认证密码
	UUID string `json:"uuid"`
	// SpeedLimit 是用户的速度限制（Mbps），nil 表示不限速
	SpeedLimit *uint32 `json:"speed_limit"`
}

// userListResponse 是用户列表 API 的响应体结构
type userListResponse struct {
	Users []User `json:"users"`
}

// GetUserList 从 V2board 面板获取当前节点的有效用户列表
func (c *Client) GetUserList() ([]User, error) {
	apiURL := c.buildURL("/api/v1/server/UniProxy/user")
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求用户列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("用户列表 API 返回非 200 状态码: %d", resp.StatusCode)
	}

	var responseData userListResponse
	if err = json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("解析用户列表 JSON 失败: %w", err)
	}

	return responseData.Users, nil
}

// ---- 流量上报 ----

// TrafficRecord 是单个用户的流量上报记录
type TrafficRecord struct {
	// UserID 是用户的数字 ID
	UserID int `json:"user_id"`
	// Upload 是用户的上行字节数（客户端 -> 服务器 -> 目标）
	Upload int64 `json:"u"`
	// Download 是用户的下行字节数（目标 -> 服务器 -> 客户端）
	Download int64 `json:"d"`
}

// PushTraffic 将用户流量数据上报给 V2board 面板
// records 中应只包含流量不为零的用户记录
func (c *Client) PushTraffic(records []TrafficRecord) error {
	if len(records) == 0 {
		return nil
	}

	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("序列化流量数据失败: %w", err)
	}

	apiURL := c.buildURL("/api/v1/server/UniProxy/push")
	resp, err := c.httpClient.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("上报流量失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("流量上报 API 返回非 200 状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}
