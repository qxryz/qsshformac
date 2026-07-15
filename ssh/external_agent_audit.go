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
	Revoked     bool   `json:"revoked"`
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

// ExternalAgentAccountPermissions 是账号权限的按需只读快照。
// SudoMode 为 none、limited、full 或 unknown。
type ExternalAgentAccountPermissions struct {
	Username  string   `json:"username"`
	Groups    []string `json:"groups"`
	SudoMode  string   `json:"sudoMode"`
	SudoRules []string `json:"sudoRules"`
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
  for state_file in "active:$f" "revoked:${f}.qssh-revoked"; do
    state="${state_file%%:*}"
    key_file="${state_file#*:}"
    [ -r "$key_file" ] || continue
    while IFS= read -r line; do
      printf '__QSSH_EXTERNAL_KEY__\t%s\t%s\t%s\n' "$state" "$f" "$line"
    done < "$key_file"
  done
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
			parts := strings.SplitN(line, "\t", 4)
			if len(parts) < 3 {
				continue
			}
			state, path, keyLine := "active", parts[1], parts[2]
			if len(parts) == 4 {
				state, path, keyLine = parts[1], parts[2], parts[3]
			}
			pub, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(strings.TrimSpace(keyLine)))
			if err != nil {
				continue
			}
			username := usernameFromAuthorizedKeysPath(path)
			audit.Keys = append(audit.Keys, ExternalAgentKey{
				Username:    username,
				Algorithm:   pub.Type(),
				Fingerprint: gossh.FingerprintSHA256(pub),
				Comment:     strings.TrimSpace(comment),
				IsRoot:      username == "root",
				Revoked:     state == "revoked",
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

const externalAgentGroupsMarker = "__QSSH_ACCOUNT_GROUPS__"
const externalAgentSudoMarker = "__QSSH_ACCOUNT_SUDO__"

var externalAgentSudoRulePattern = regexp.MustCompile(`^\(([^)]*)\)[[:space:]]+(?:(NOPASSWD|PASSWD):[[:space:]]*)?(.+)$`)

// GetExternalAgentAccountPermissions 按需读取指定账号的用户组与 sudo 规则。
// root 可以检查所有账号，普通连接只能检查当前登录账号。
func (s *SSHService) GetExternalAgentAccountPermissions(connID, username string) (*ExternalAgentAccountPermissions, error) {
	if !safeRemoteUsername.MatchString(username) {
		return nil, fmt.Errorf("用户名格式不安全")
	}

	s.mu.RLock()
	client, exists := s.clients[connID]
	s.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("连接不存在: %s", connID)
	}
	currentUser := ""
	if client.config != nil {
		currentUser = client.config.Username
	}
	if !canRevokeExternalAgentKeyForUser(currentUser, username) {
		return nil, fmt.Errorf("当前 SSH 登录账号（%s）无权检查 %s 的权限", currentUser, username)
	}

	var sudoCommand string
	if currentUser == "root" {
		sudoCommand = fmt.Sprintf(`sudo -n -l -U %s 2>&1 || true`, shellSingleQuote(username))
	} else {
		sudoCommand = `sudo -n -l 2>&1 || true`
	}
	command := fmt.Sprintf(`set +e
printf '%s\t'
id -nG %s 2>/dev/null || true
printf '%s\n'
if command -v sudo >/dev/null 2>&1; then %s; else printf 'sudo command is not installed\n'; fi`,
		externalAgentGroupsMarker, shellSingleQuote(username), externalAgentSudoMarker, sudoCommand,
	)
	result, err := client.ExecuteCommand(command)
	if err != nil || result == nil || !result.Success {
		return nil, fmt.Errorf("读取 %s 的账号权限失败", username)
	}
	permissions := parseExternalAgentAccountPermissions(username, result.Stdout)
	return &permissions, nil
}

func parseExternalAgentAccountPermissions(username, output string) ExternalAgentAccountPermissions {
	permissions := ExternalAgentAccountPermissions{
		Username:  username,
		Groups:    []string{},
		SudoMode:  "unknown",
		SudoRules: []string{},
	}
	inSudo := false
	sudoText := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, externalAgentGroupsMarker+"\t") {
			permissions.Groups = strings.Fields(strings.TrimPrefix(line, externalAgentGroupsMarker+"\t"))
			continue
		}
		if line == externalAgentSudoMarker {
			inSudo = true
			continue
		}
		if !inSudo {
			continue
		}
		sudoText += line + "\n"
		trimmed := strings.TrimSpace(line)
		matches := externalAgentSudoRulePattern.FindStringSubmatch(trimmed)
		if len(matches) != 4 {
			continue
		}
		rule := strings.TrimSpace(matches[3])
		if rule == "" {
			continue
		}
		prefix := "(" + strings.TrimSpace(matches[1]) + ") "
		if matches[2] != "" {
			prefix += matches[2] + ": "
		}
		permissions.SudoRules = append(permissions.SudoRules, prefix+rule)
		if rule == "ALL" {
			permissions.SudoMode = "full"
		}
	}

	if username == "root" {
		permissions.SudoMode = "full"
		if len(permissions.SudoRules) == 0 {
			permissions.SudoRules = []string{"root 固有完整系统权限"}
		}
		return permissions
	}
	if permissions.SudoMode != "full" && len(permissions.SudoRules) > 0 {
		permissions.SudoMode = "limited"
		return permissions
	}
	lower := strings.ToLower(sudoText)
	if strings.Contains(lower, "not allowed to run sudo") ||
		strings.Contains(lower, "may not run sudo") ||
		strings.Contains(lower, "is not in the sudoers file") ||
		strings.Contains(lower, "sudo command is not installed") {
		permissions.SudoMode = "none"
	}
	return permissions
}

// RevokeExternalAgentKey 按指纹暂停指定账号的授权，并保留可恢复副本。
// root 可以管理所有账号；处理普通账号时会降权到目标账号执行文件操作，
// 避免 root 直接写入低权限用户可控制的目录。
func (s *SSHService) RevokeExternalAgentKey(connID, username, fingerprint string) error {
	return s.changeExternalAgentKeyState(connID, username, fingerprint, "revoke")
}

// RestoreExternalAgentKey 将已撤销区中的公钥恢复到 authorized_keys。
func (s *SSHService) RestoreExternalAgentKey(connID, username, fingerprint string) error {
	return s.changeExternalAgentKeyState(connID, username, fingerprint, "restore")
}

// PermanentlyRevokeExternalAgentKey 从授权文件和可恢复区彻底删除公钥。
func (s *SSHService) PermanentlyRevokeExternalAgentKey(connID, username, fingerprint string) error {
	return s.changeExternalAgentKeyState(connID, username, fingerprint, "permanent")
}

func (s *SSHService) changeExternalAgentKeyState(connID, username, fingerprint, action string) error {
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
		return fmt.Errorf("当前 SSH 登录账号（%s）无权管理 %s 的密钥", currentUser, username)
	}

	readCommand := externalAgentCommandAsUser(currentUser, username, authorizedKeysStateReadScript())
	readResult, err := client.ExecuteCommand(readCommand)
	if err != nil || readResult == nil || !readResult.Success {
		return fmt.Errorf("读取账号密钥状态失败")
	}

	active, revoked := splitAuthorizedKeyState(readResult.Stdout)
	activeAfter, activeLine, activeFound := takeAuthorizedKeyByFingerprint(active, fingerprint)
	revokedAfter, revokedLine, revokedFound := takeAuthorizedKeyByFingerprint(revoked, fingerprint)
	switch action {
	case "revoke":
		if !activeFound {
			return fmt.Errorf("未找到可撤销的授权，可能已经撤销")
		}
		active = activeAfter
		if !revokedFound {
			revoked = appendAuthorizedKeyLine(revoked, activeLine)
		}
	case "restore":
		if !revokedFound {
			return fmt.Errorf("未找到可恢复的公钥")
		}
		revoked = revokedAfter
		if !activeFound {
			active = appendAuthorizedKeyLine(active, revokedLine)
		}
	case "permanent":
		if !activeFound && !revokedFound {
			return fmt.Errorf("未找到对应公钥")
		}
		active, revoked = activeAfter, revokedAfter
	default:
		return fmt.Errorf("不支持的密钥操作")
	}

	encodedActive := base64.StdEncoding.EncodeToString([]byte(active))
	encodedRevoked := base64.StdEncoding.EncodeToString([]byte(revoked))
	expectedActiveHash := fmt.Sprintf("%x", sha256.Sum256([]byte(splitAuthorizedKeyStatePart(readResult.Stdout, externalAgentActiveStateMarker))))
	expectedRevokedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(splitAuthorizedKeyStatePart(readResult.Stdout, externalAgentRevokedStateMarker))))
	writeScript := fmt.Sprintf(
		`set -eu
dir="$HOME/.ssh"
path="$dir/authorized_keys"
revoked="$dir/authorized_keys.qssh-revoked"
[ ! -e "$path.qssh.bak" ] || [ ! -L "$path.qssh.bak" ] || exit 1
[ ! -e "$revoked.qssh.bak" ] || [ ! -L "$revoked.qssh.bak" ] || exit 1
[ -d "$dir" ] && [ ! -L "$dir" ] && [ -f "$path" ] && [ ! -L "$path" ] || exit 1
[ ! -e "$revoked" ] || { [ -f "$revoked" ] && [ ! -L "$revoked" ]; } || exit 1
hash_file() { if [ -f "$1" ]; then if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; else printf '%%s' %s; fi; }
[ "$(hash_file "$path")" = %s ] || exit 1
[ "$(hash_file "$revoked")" = %s ] || exit 1
active_tmp="$(mktemp "$dir/.authorized_keys.qssh.XXXXXX")"
revoked_tmp="$(mktemp "$dir/.authorized_keys-revoked.qssh.XXXXXX")"
trap 'rm -f "$active_tmp" "$revoked_tmp"' EXIT
printf '%%s' %s | (base64 -d 2>/dev/null || base64 -D) > "$active_tmp"
printf '%%s' %s | (base64 -d 2>/dev/null || base64 -D) > "$revoked_tmp"
chmod 600 "$active_tmp" "$revoked_tmp"
cp -p "$path" "$path.qssh.bak"
[ ! -f "$revoked" ] || cp -p "$revoked" "$revoked.qssh.bak"
mv -f "$active_tmp" "$path"
if [ -s "$revoked_tmp" ]; then mv -f "$revoked_tmp" "$revoked"; else rm -f "$revoked" "$revoked_tmp"; fi
trap - EXIT`,
		shellSingleQuote(fmt.Sprintf("%x", sha256.Sum256(nil))),
		shellSingleQuote(expectedActiveHash), shellSingleQuote(expectedRevokedHash),
		shellSingleQuote(encodedActive), shellSingleQuote(encodedRevoked),
	)
	writeCommand := externalAgentCommandAsUser(currentUser, username, writeScript)
	writeResult, err := client.ExecuteCommand(writeCommand)
	if err != nil || writeResult == nil || !writeResult.Success {
		return fmt.Errorf("更新密钥状态失败")
	}
	return nil
}

const externalAgentActiveStateMarker = "__QSSH_ACTIVE_KEYS__"
const externalAgentRevokedStateMarker = "__QSSH_REVOKED_KEYS__"

func authorizedKeysStateReadScript() string {
	return `set -eu
path="$HOME/.ssh/authorized_keys"
revoked="$HOME/.ssh/authorized_keys.qssh-revoked"
[ -r "$path" ] && [ ! -L "$path" ] || exit 1
printf '__QSSH_ACTIVE_KEYS__\t'
(base64 < "$path" | tr -d '\n')
printf '\n__QSSH_REVOKED_KEYS__\t'
if [ -r "$revoked" ] && [ ! -L "$revoked" ]; then base64 < "$revoked" | tr -d '\n'; fi
printf '\n'`
}

func splitAuthorizedKeyState(output string) (string, string) {
	return splitAuthorizedKeyStatePart(output, externalAgentActiveStateMarker), splitAuthorizedKeyStatePart(output, externalAgentRevokedStateMarker)
}

func splitAuthorizedKeyStatePart(output, marker string) string {
	for _, line := range strings.Split(output, "\n") {
		prefix := marker + "\t"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		if err == nil {
			return string(decoded)
		}
	}
	return ""
}

func takeAuthorizedKeyByFingerprint(content, fingerprint string) (string, string, bool) {
	kept := make([]string, 0)
	matchedLine := ""
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(trimmed))
		if err == nil && gossh.FingerprintSHA256(pub) == fingerprint {
			if matchedLine == "" {
				matchedLine = strings.TrimRight(line, "\r\n")
			}
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, ""), matchedLine, matchedLine != ""
}

func appendAuthorizedKeyLine(content, line string) string {
	if strings.TrimSpace(content) == "" {
		return line + "\n"
	}
	return strings.TrimRight(content, "\r\n") + "\n" + line + "\n"
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
