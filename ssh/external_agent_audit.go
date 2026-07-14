package ssh

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// GetExternalAgentScopeID 返回稳定的服务器级监管作用域 ID。
// 同一服务器切换 root 和专用用户后仍使用同一份监管记录。
func (s *SSHService) GetExternalAgentScopeID(connID string) (string, error) {
	conn, err := s.storage.GetConnection(connID)
	if err != nil {
		return "", err
	}
	raw := fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(conn.Host)), conn.Port)
	sum := sha256.Sum256([]byte(raw))
	return "external-agent-scope-" + hex.EncodeToString(sum[:12]), nil
}

// ExternalAgentKey 是远端 authorized_keys 的脱敏视图。
// 公钥正文永远不会通过 Wails binding 返回前端。
type ExternalAgentKey struct {
	Username    string `json:"username"`
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
	IsRoot      bool   `json:"isRoot"`
}

// ExternalAgentSession 描述当前可观察到的 SSH 登录会话。
// 这只能证明账号已登录，不能推断外部 Agent 正在执行的具体任务。
type ExternalAgentSession struct {
	Username    string `json:"username"`
	Terminal    string `json:"terminal"`
	LoginAt     string `json:"loginAt"`
	Source      string `json:"source"`
	AuthMethod  string `json:"authMethod"`
	Fingerprint string `json:"fingerprint"`
	Active      bool   `json:"active"`
	PID         int    `json:"pid"`
}

// ExternalAgentAudit 是外部 Agent 接管监管面板使用的安全快照。
type ExternalAgentAudit struct {
	Keys          []ExternalAgentKey     `json:"keys"`
	Sessions      []ExternalAgentSession `json:"sessions"`
	CanInspectAll bool                   `json:"canInspectAll"`
	ScannedAt     string                 `json:"scannedAt"`
}

const externalAgentKeyMarker = "__QSSH_EXTERNAL_KEY__"
const externalAgentWhoMarker = "__QSSH_EXTERNAL_WHO__"
const externalAgentAuthMarker = "__QSSH_EXTERNAL_AUTH__"
const externalAgentProcessMarker = "__QSSH_EXTERNAL_PROCESS__"
const externalAgentSelfProcessMarker = "__QSSH_EXTERNAL_SELF_PROCESS__"

var acceptedSSHLoginPattern = regexp.MustCompile(`Accepted ([^ ]+) for (?:invalid user )?([^ ]+) from ([^ ]+) port [0-9]+(?: ssh2(?:: [^ ]+ (SHA256:[^ ]+))?)?`)
var activeSSHProcessPattern = regexp.MustCompile(`^[[:space:]]*([0-9]+)[[:space:]]+([0-9]+)[[:space:]]+sshd: ([^ ]+)@([^ ]+)[[:space:]]*$`)

// GetExternalAgentAudit 扫描 root 与常见专用用户的授权密钥，并读取当前登录会话。
// 扫描只读；返回值只包含算法、标签和指纹，不包含公钥正文。
func (s *SSHService) GetExternalAgentAudit(connID string) (*ExternalAgentAudit, error) {
	s.mu.RLock()
	client, exists := s.clients[connID]
	s.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("连接不存在: %s", connID)
	}

	command := `set +e
if [ "$(id -u)" = "0" ]; then
  printf '__QSSH_EXTERNAL_SCOPE__\tall\n'
  files="/root/.ssh/authorized_keys"
  for home in /home/*; do [ -d "$home" ] && files="$files $home/.ssh/authorized_keys"; done
else
  printf '__QSSH_EXTERNAL_SCOPE__\tself\n'
  files="$HOME/.ssh/authorized_keys"
fi
for f in $files; do
  [ -r "$f" ] || continue
  while IFS= read -r line; do
    printf '__QSSH_EXTERNAL_KEY__\t%s\t%s\n' "$f" "$line"
  done < "$f"
done
printf '__QSSH_EXTERNAL_WHO__\n'
who 2>/dev/null || true
printf '__QSSH_EXTERNAL_SELF_PROCESS__\t%s\n' "$PPID"
printf '__QSSH_EXTERNAL_PROCESS__\n'
ps -eo pid=,etimes=,args= 2>/dev/null | grep 'sshd: .*@' || true
printf '__QSSH_EXTERNAL_AUTH__\n'
if [ "$(id -u)" = "0" ]; then
  if command -v journalctl >/dev/null 2>&1; then
		(journalctl -u ssh -u sshd --since '-24 hours' --no-pager -o short-iso 2>/dev/null || true) | grep 'Accepted ' | tail -n 500
  elif [ -r /var/log/auth.log ]; then
    grep 'Accepted ' /var/log/auth.log | tail -n 500
  elif [ -r /var/log/secure ]; then
    grep 'Accepted ' /var/log/secure | tail -n 500
  fi
fi`

	result, err := client.ExecuteCommand(command)
	if err != nil {
		return nil, fmt.Errorf("扫描外部 Agent 接管状态失败: %w", err)
	}
	if result == nil || !result.Success {
		return nil, fmt.Errorf("扫描外部 Agent 接管状态失败")
	}

	audit := parseExternalAgentAudit(result.Stdout)
	return &audit, nil
}

func parseExternalAgentAudit(output string) ExternalAgentAudit {
	audit := ExternalAgentAudit{
		Keys:      []ExternalAgentKey{},
		Sessions:  []ExternalAgentSession{},
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
	}
	inWho := false
	inProcess := false
	inAuth := false
	type loginEvidence struct {
		method      string
		fingerprint string
	}
	evidence := make(map[string]loginEvidence)
	recentByIdentity := make(map[string]ExternalAgentSession)
	selfProcessPIDs := make(map[int]struct{})
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "__QSSH_EXTERNAL_SCOPE__\t") {
			audit.CanInspectAll = strings.TrimSpace(strings.TrimPrefix(line, "__QSSH_EXTERNAL_SCOPE__\t")) == "all"
			continue
		}
		if strings.HasPrefix(line, externalAgentSelfProcessMarker+"\t") {
			pid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, externalAgentSelfProcessMarker+"\t")))
			if err == nil && pid > 0 {
				selfProcessPIDs[pid] = struct{}{}
			}
			continue
		}
		if line == externalAgentWhoMarker {
			inWho = true
			inProcess = false
			inAuth = false
			continue
		}
		if line == externalAgentProcessMarker {
			inWho = false
			inProcess = true
			inAuth = false
			continue
		}
		if line == externalAgentAuthMarker {
			inWho = false
			inProcess = false
			inAuth = true
			continue
		}
		if strings.HasPrefix(line, externalAgentKeyMarker+"\t") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) != 3 {
				continue
			}
			pub, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(strings.TrimSpace(parts[2])))
			if err != nil {
				continue
			}
			username := usernameFromAuthorizedKeysPath(parts[1])
			audit.Keys = append(audit.Keys, ExternalAgentKey{
				Username:    username,
				Algorithm:   pub.Type(),
				Fingerprint: gossh.FingerprintSHA256(pub),
				Comment:     strings.TrimSpace(comment),
				IsRoot:      username == "root",
			})
			continue
		}
		if inAuth {
			matches := acceptedSSHLoginPattern.FindStringSubmatch(line)
			if len(matches) == 5 {
				login := loginEvidence{
					method:      matches[1],
					fingerprint: matches[4],
				}
				evidence[matches[2]+"\x00"+matches[3]] = login
				observedAt := strings.TrimSpace(line[:strings.Index(line, "Accepted ")])
				recentByIdentity[matches[2]+"\x00"+matches[4]+"\x00"+matches[1]] = ExternalAgentSession{
					Username: matches[2], Fingerprint: matches[4], AuthMethod: matches[1],
					Source: matches[3], LoginAt: observedAt, Terminal: "recent-auth", Active: false,
				}
			}
			continue
		}
		if inProcess {
			matches := activeSSHProcessPattern.FindStringSubmatch(line)
			if len(matches) != 5 {
				continue
			}
			pid, _ := strconv.Atoi(matches[1])
			if _, isSelfProcess := selfProcessPIDs[pid]; isSelfProcess {
				continue
			}
			merged := false
			for index := range audit.Sessions {
				// `who` 不会列出无来源的 notty 会话。不要把它和其他
				// 同用户名会话合并，否则会为无法确认的会话继承认证证据。
				if matches[4] != "notty" && audit.Sessions[index].Username == matches[3] && audit.Sessions[index].Terminal == matches[4] {
					audit.Sessions[index].PID = pid
					audit.Sessions[index].Active = true
					merged = true
					break
				}
			}
			if !merged {
				audit.Sessions = append(audit.Sessions, ExternalAgentSession{
					Username: matches[3], Terminal: matches[4], LoginAt: "当前在线", Active: true, PID: pid,
				})
			}
			continue
		}
		if inWho && strings.TrimSpace(line) != "" {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			source := ""
			if len(fields) >= 5 {
				source = strings.Trim(fields[len(fields)-1], "()")
			}
			audit.Sessions = append(audit.Sessions, ExternalAgentSession{
				Username: fields[0],
				Terminal: fields[1],
				LoginAt:  fields[2] + " " + fields[3],
				Source:   source,
				Active:   true,
			})
		}
	}
	for index := range audit.Sessions {
		session := &audit.Sessions[index]
		if session.Source == "" {
			continue
		}
		if match, ok := evidence[session.Username+"\x00"+session.Source]; ok {
			session.AuthMethod = match.method
			session.Fingerprint = match.fingerprint
		}
	}
	for _, recent := range recentByIdentity {
		audit.Sessions = append(audit.Sessions, recent)
	}
	return audit
}

func usernameFromAuthorizedKeysPath(path string) string {
	if strings.HasPrefix(path, "/root/") {
		return "root"
	}
	if strings.HasPrefix(path, "/home/") {
		rest := strings.TrimPrefix(path, "/home/")
		if idx := strings.IndexByte(rest, '/'); idx > 0 {
			return rest[:idx]
		}
	}
	return "current-user"
}

var safeRemoteUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// RevokeExternalAgentKey 按指纹撤销指定账号的授权密钥。
// root 可以管理所有账号；处理普通账号时会降权到目标账号执行文件操作，
// 避免 root 直接写入低权限用户可控制的目录。
func (s *SSHService) RevokeExternalAgentKey(connID, username, fingerprint string) error {
	if !safeRemoteUsername.MatchString(username) {
		return fmt.Errorf("用户名格式不安全")
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		return fmt.Errorf("密钥指纹格式无效")
	}

	s.mu.RLock()
	client, exists := s.clients[connID]
	s.mu.RUnlock()
	if !exists {
		return fmt.Errorf("连接不存在: %s", connID)
	}
	currentUser := ""
	if client.config != nil {
		currentUser = client.config.Username
	}
	if !canRevokeExternalAgentKeyForUser(currentUser, username) {
		return fmt.Errorf("当前 SSH 登录账号（%s）无权撤销 %s 的密钥", currentUser, username)
	}

	readCommand := externalAgentCommandAsUser(currentUser, username, authorizedKeysReadScript())
	readResult, err := client.ExecuteCommand(readCommand)
	if err != nil || readResult == nil || !readResult.Success {
		return fmt.Errorf("读取 authorized_keys 失败")
	}

	content, removed := removeAuthorizedKeyByFingerprint(readResult.Stdout, fingerprint)
	if !removed {
		return fmt.Errorf("未找到对应密钥，可能已被撤销")
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(readResult.Stdout)))
	writeScript := fmt.Sprintf(
		`set -eu
dir="$HOME/.ssh"
path="$dir/authorized_keys"
backup="$dir/authorized_keys.qssh.bak"
[ -d "$dir" ] && [ ! -L "$dir" ] && [ -f "$path" ] && [ ! -L "$path" ] || exit 1
[ ! -e "$backup" ] || [ ! -L "$backup" ] || exit 1
if command -v sha256sum >/dev/null 2>&1; then current_hash="$(sha256sum "$path" | awk '{print $1}')"; else current_hash="$(shasum -a 256 "$path" | awk '{print $1}')"; fi
[ "$current_hash" = %s ] || exit 1
tmp="$(mktemp "$dir/.authorized_keys.qssh.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
if ! printf '%%s' %s | (base64 -d 2>/dev/null || base64 -D) > "$tmp"; then exit 1; fi
chmod 600 "$tmp"
cp -p "$path" "$backup"
mv -f "$tmp" "$path"
trap - EXIT`,
		shellSingleQuote(expectedHash), shellSingleQuote(encoded),
	)
	writeCommand := externalAgentCommandAsUser(currentUser, username, writeScript)
	writeResult, err := client.ExecuteCommand(writeCommand)
	if err != nil || writeResult == nil || !writeResult.Success {
		return fmt.Errorf("撤销密钥失败")
	}
	return nil
}

func removeAuthorizedKeyByFingerprint(content, fingerprint string) (string, bool) {
	removed := false
	kept := make([]string, 0)
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		pub, _, _, _, parseErr := gossh.ParseAuthorizedKey([]byte(trimmed))
		if parseErr == nil && gossh.FingerprintSHA256(pub) == fingerprint {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, ""), removed
}

func canRevokeExternalAgentKeyForUser(currentUser, targetUser string) bool {
	return currentUser != "" && (currentUser == targetUser || currentUser == "root")
}

func authorizedKeysReadScript() string {
	return `set -eu
path="$HOME/.ssh/authorized_keys"
[ -r "$path" ] && [ ! -L "$path" ] || exit 1
cat "$path"`
}

func externalAgentCommandAsUser(currentUser, targetUser, script string) string {
	if currentUser == targetUser {
		return script
	}
	return fmt.Sprintf(`set -eu
target=%s
home="$(getent passwd "$target" | awk -F: 'NR == 1 { print $6; exit }')"
case "$home" in /*) ;; *) exit 1 ;; esac
command -v runuser >/dev/null 2>&1 || exit 1
exec runuser -u "$target" -- env HOME="$home" sh -c %s`, shellSingleQuote(targetUser), shellSingleQuote(script))
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
