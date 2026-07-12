package ssh

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"changeme/apppaths"
	"golang.org/x/crypto/ssh"
)

// hostKeyStore 以 TOFU（首次信任）方式管理 SSH 主机密钥指纹。
// 首次连接某主机时记录其公钥指纹，之后必须匹配，否则拒绝连接，
// 防止中间人攻击。替换原来的 ssh.InsecureIgnoreHostKey()。
var (
	hostKeyMu   sync.Mutex
	hostKeyFile string
)

func knownHostsPath() string {
	if hostKeyFile == "" {
		hostKeyFile = filepath.Join(apppaths.SubDir("config"), "known_hosts.json")
	}
	return hostKeyFile
}

func loadKnownHosts() map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile(knownHostsPath())
	if err == nil {
		json.Unmarshal(data, &m)
	}
	return m
}

func saveKnownHosts(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(knownHostsPath(), data, 0600)
}

// keyFingerprint 返回公钥的 SHA-256 指纹（OpenSSH 风格）。
func keyFingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// tofuHostKeyCallback 返回一个 TOFU 主机密钥校验回调。
// hostID 用作存储键（通常为 host:port）。
func tofuHostKeyCallback(hostID string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return verifyHostKey(hostID, key)
	}
}

func verifyHostKey(hostID string, key ssh.PublicKey) error {
	fp := keyFingerprint(key)

	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()

	known := loadKnownHosts()
	if saved, ok := known[hostID]; ok {
		if saved != fp {
			return fmt.Errorf("主机密钥不匹配（可能存在中间人攻击）\n主机: %s\n已保存指纹: %s\n本次指纹: %s\n如确认服务器密钥已变更，请删除 known_hosts.json 中的对应记录", hostID, saved, fp)
		}
		return nil
	}

	// 首次连接：记录指纹（TOFU）
	known[hostID] = fp
	if err := saveKnownHosts(known); err != nil {
		fmt.Printf("[SSH] ⚠️ 保存主机密钥指纹失败: %v\n", err)
	} else {
		fmt.Printf("[SSH] 已信任并记录主机密钥: %s %s\n", hostID, fp)
	}
	return nil
}
