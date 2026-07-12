package client

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message 协议消息
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// SyncConnection 同步的连接配置
type SyncConnection struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"password,omitempty"`
	KeyPath   string    `json:"keyPath,omitempty"`
	Source    string    `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// SyncData 同步数据
type SyncData struct {
	Connections []SyncConnection `json:"connections"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	DeviceID    string           `json:"deviceId"`
}

// CloudClient 云端客户端
type CloudClient struct {
	mu         sync.RWMutex
	serverAddr string
	token      string
	deviceID   string
	conn       *websocket.Conn
	connected  bool
}

// NewCloudClient 创建云端客户端
func NewCloudClient(serverAddr, token string) *CloudClient {
	return &CloudClient{
		serverAddr: serverAddr,
		token:      token,
	}
}

// Connect 连接到云端服务器
func (cc *CloudClient) Connect() error {
	u := url.URL{
		Scheme: "wss",
		Host:   cc.serverAddr,
		Path:   "/ws",
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			// 服务端使用自签名证书，无法走标准 CA 校验。
			// 采用 TOFU（首次信任）证书固定：首次连接记录证书指纹，
			// 之后必须匹配，防止中间人攻击。
			InsecureSkipVerify:    true, // 由 VerifyConnection 接管校验
			VerifyConnection:      cc.verifyPinnedCert,
		},
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %v", err)
	}

	cc.mu.Lock()
	cc.conn = conn
	cc.connected = true
	cc.mu.Unlock()

	return nil
}

// verifyPinnedCert 实现 TOFU 证书固定校验。
func (cc *CloudClient) verifyPinnedCert(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return fmt.Errorf("服务端未提供证书")
	}
	sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
	fp := hex.EncodeToString(sum[:])

	pinned, err := loadPinnedFingerprint(cc.serverAddr)
	if err == nil && pinned != "" {
		if pinned != fp {
			return fmt.Errorf("证书指纹不匹配（可能存在中间人攻击）：期望 %s，实际 %s", pinned, fp)
		}
		return nil
	}
	// 首次连接：记录指纹
	if err := savePinnedFingerprint(cc.serverAddr, fp); err != nil {
		fmt.Printf("[CloudClient] ⚠️ 保存证书指纹失败: %v\n", err)
	} else {
		fmt.Printf("[CloudClient] 已固定服务端证书指纹: %s\n", fp)
	}
	return nil
}

// pinFilePath 返回证书指纹存储文件路径。
func pinFilePath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "qssh", "cloud")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "known_certs.json")
}

func loadPins() map[string]string {
	pins := map[string]string{}
	data, err := os.ReadFile(pinFilePath())
	if err == nil {
		json.Unmarshal(data, &pins)
	}
	return pins
}

func loadPinnedFingerprint(addr string) (string, error) {
	pins := loadPins()
	if fp, ok := pins[addr]; ok {
		return fp, nil
	}
	return "", fmt.Errorf("未找到指纹")
}

func savePinnedFingerprint(addr, fp string) error {
	pins := loadPins()
	pins[addr] = fp
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pinFilePath(), data, 0600)
}

// Disconnect 断开连接
func (cc *CloudClient) Disconnect() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.conn != nil {
		cc.conn.Close()
		cc.conn = nil
	}
	cc.connected = false
}

// IsConnected 检查是否连接
func (cc *CloudClient) IsConnected() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.connected && cc.conn != nil
}

// send 发送消息并接收响应
func (cc *CloudClient) send(msg Message) (Message, error) {
	cc.mu.RLock()
	conn := cc.conn
	connected := cc.connected
	cc.mu.RUnlock()

	if conn == nil || !connected {
		return Message{}, fmt.Errorf("未连接")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return Message{}, fmt.Errorf("序列化失败: %v", err)
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		cc.Disconnect()
		return Message{}, fmt.Errorf("发送失败: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, respData, err := conn.ReadMessage()
	if err != nil {
		cc.Disconnect()
		return Message{}, fmt.Errorf("接收失败: %v", err)
	}

	var resp Message
	if err := json.Unmarshal(respData, &resp); err != nil {
		return Message{}, fmt.Errorf("解析失败: %v", err)
	}

	return resp, nil
}

// Register 注册设备
func (cc *CloudClient) Register(deviceName, host, osInfo, version string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"name":      deviceName,
		"host":      host,
		"port":      0,
		"os":        osInfo,
		"version":   version,
		"token":     cc.token,
		"timestamp": time.Now().Format(time.RFC3339),
		"nonce":     generateNonce(),
	})

	resp, err := cc.send(Message{Type: "register", Payload: payload})
	if err != nil {
		return err
	}

	if resp.Type == "error" {
		return fmt.Errorf("注册失败: %s", string(resp.Payload))
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Payload, &result)
	if id, ok := result["id"].(string); ok {
		cc.deviceID = id
	}

	return nil
}

// Heartbeat 发送心跳
func (cc *CloudClient) Heartbeat() error {
	payload, _ := json.Marshal(map[string]string{"deviceId": cc.deviceID})
	_, err := cc.send(Message{Type: "heartbeat", Payload: payload})
	return err
}

// PullSync 拉取同步数据
func (cc *CloudClient) PullSync() (*SyncData, error) {
	resp, err := cc.send(Message{Type: "sync-pull", Payload: json.RawMessage("{}")})
	if err != nil {
		return nil, err
	}

	if resp.Type == "error" {
		return nil, fmt.Errorf("拉取失败: %s", string(resp.Payload))
	}

	var data SyncData
	json.Unmarshal(resp.Payload, &data)
	return &data, nil
}

// PushSync 推送同步数据
func (cc *CloudClient) PushSync(data SyncData) error {
	data.DeviceID = cc.deviceID
	payload, _ := json.Marshal(data)

	resp, err := cc.send(Message{Type: "sync-push", Payload: payload})
	if err != nil {
		return err
	}

	if resp.Type == "error" {
		return fmt.Errorf("推送失败: %s", string(resp.Payload))
	}

	return nil
}

func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 极少发生；退化为时间派生，仅作兜底
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (8 * (i % 8)))
		}
	}
	return fmt.Sprintf("%x", b)
}
