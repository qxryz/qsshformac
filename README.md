# 舟SSH（qssh 0.3.2.1 for mac）

舟SSH是一款面向 macOS 的中文 SSH 客户端，提供终端、SFTP、端口转发、服务器管理和 AI 辅助操作，并新增了面向外部工作机 Agent 的 SSH 密钥监管能力。

本项目移植自 [nanxiangxi/qssh](https://github.com/nanxiangxi/qssh)，在保留原有功能的基础上完成 macOS 适配，并将应用更名为「舟SSH」、内置 AI 助手命名为「舟舟」。

## 功能概览

- 多标签 SSH 终端，支持经典 xterm 模式和结构化模式
- SFTP 文件管理：上传、下载和在线编辑
- 连接保存、快速连接、分组与云端同步
- 本地与远程端口转发
- 批量命令执行与操作日志
- 防火墙管理：iptables、firewalld、ufw
- systemd 服务与进程守护管理
- 内置 AI 助手：支持 OpenAI 兼容 API，并可在用户授权下执行远端命令
- 外部 Agent SSH 密钥监管：按指纹归类账号、撤销、恢复和彻底撤销权限

## macOS 适配

- 打包为标准 `.app`，支持 Apple Silicon（arm64）和 Intel（amd64）
- 数据目录使用 `~/Library/Application Support/qssh`
- 支持 `⌘+C/V`、`⌘+←/→`、`⌘+Shift+S/U/D` 等 macOS 快捷键
- 终端中的 `Ctrl+C` 仍保持 Unix 中断语义
- 终端和编辑器优先使用 SF Mono、Menlo、Monaco
- Windows 系统托盘适配为 macOS 菜单栏图标
- 云端同步按实际 macOS 平台注册设备

## 外部 Agent SSH 密钥监管

“外部 Agent”指运行在另一台电脑上的 Codex 或其他自动化 Agent。它使用自己工作机上的 SSH 私钥连接服务器，与舟SSH内置 AI 助手并不是同一个主体。

适用范围：当前外部 Agent 密钥监管功能仅面向使用 OpenSSH 的 Linux 服务器（例如 Ubuntu、Debian、CentOS 等）；完整扫描和跨账号撤销需要使用用户名为 `root` 的 SSH 连接，不适用于将 macOS 或 Windows 个人电脑作为被监管目标。

舟SSH右侧工具栏中的“密钥管理”面板是一张按指纹组织的密钥管理卡，可以：

- 按完整 SHA-256 指纹归类该密钥可以登录的所有账号
- 为每个“指纹 + 账号”单独显示可登录或已撤销状态
- 某个账号通过对应指纹在线时，显示指纹确认的在线会话数量、终端和脱敏来源
- 点击账号的“权限”后按需显示所属用户组、无 sudo/受限 sudo/完整 sudo 状态和精确允许命令
- 非 root 账号在检查前只标记为“权限待检查”；检查后再归类为低权限、受限运维或管理员账号，避免把拥有 sudo 的 `admin` 误标为普通用户
- 临时撤销公钥并保留服务器端可恢复副本，也可一键恢复
- 从当前授权和可恢复区彻底撤销公钥；彻底撤销后只能重新安装
- 保留 Agent 接管提示词和工作机私钥彻底删除提示词，各自一键复制
- 通用接管提示词允许每位用户自行命名或随机生成名称，并明确禁止专属账号获得提权能力

### 指纹与账号

面板以完整公钥指纹作为主键，不依赖容易重复或被修改的公钥注释。同一指纹可以同时出现在 `root`、专属账号或其他普通账号下，每个账号的授权都可以独立管理。

普通“撤销”会把对应公钥从 `authorized_keys` 移入同账号的 `authorized_keys.qssh-revoked`，因此之后可以恢复；“彻底撤销”会同时删除当前授权和可恢复副本。两种操作都会在改写前保留 `.qssh.bak` 备份，并使用内容哈希避免覆盖并发修改。

`ssh-agent` 不是文件夹，而是工作机上缓存已加载私钥的后台进程；`SSH_AUTH_SOCK` 指向它的临时通信 socket。实际私钥是磁盘文件，通常位于 `~/.ssh/`。彻底删除提示词会先让每位用户核对自己的真实路径和指纹，然后从 ssh-agent 卸载单把密钥并删除磁盘上的私钥和 `.pub`，最后分别验证内存列表和文件状态。

## 推荐的安全接管流程

1. 外部 Agent 在工作机生成一把独立 Ed25519 密钥。
2. 私钥只保存在该工作机；Agent只报告公钥、标签、指纹和私钥保存路径。
3. 用户根据实际环境选择如何完成首次授权；如临时使用 root，应先核对服务器主机指纹。
4. Agent 创建专属普通账号，不加入 `sudo`、`wheel`、`admin` 等管理组，不写入 sudoers，也不授予其他提权能力。
5. Agent 将同一公钥安装到专属账号，并从新的 SSH 会话验证可以登录。
6. 用户在密钥管理卡中自行决定是否保留同指纹的 root 授权；舟SSH不强制取消 root。
7. 需要暂停接管时撤销专属账号的公钥，需要恢复时再恢复。
8. 不再需要该密钥时，先彻底撤销服务器公钥，再复制工作机私钥彻底删除提示词清理内存缓存和磁盘文件。

专属账号是 Agent 的常规登录身份；它可以登录，但不能 sudo 或以其他方式提权。

<details>
<summary>查看参考命令</summary>

在外部 Agent 的工作机生成独立密钥：

```bash
ssh-keygen \
  -t ed25519 \
  -f "$HOME/.ssh/<AGENT_KEY_NAME>" \
  -N '' \
  -C '<KEY_LABEL>'

ssh-keygen -lf "$HOME/.ssh/<AGENT_KEY_NAME>.pub"
```

用户在服务器上临时安装公钥：

```bash
install -d -m 700 /root/.ssh
printf '%s\n' 'ssh-ed25519 <PUBLIC_KEY> <KEY_LABEL>' \
  >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
```

外部 Agent 临时使用 root：

```bash
ssh -i "$HOME/.ssh/<AGENT_KEY_NAME>" root@<SERVER_HOST>
```

迁移后验证专用用户：

```bash
ssh -i "$HOME/.ssh/<AGENT_KEY_NAME>" <SERVICE_USER>@<SERVER_HOST>
```

服务器授权撤销后，在保存私钥的工作机核对并清理：

```bash
KEY_PATH="$HOME/.ssh/<AGENT_KEY_NAME>"

ssh-keygen -lf "${KEY_PATH}.pub"
ssh-add -d "${KEY_PATH}" 2>/dev/null || true
rm -i -- "${KEY_PATH}" "${KEY_PATH}.pub"
```

不要在服务器上执行最后一组工作机清理命令。删除前应再次核对服务器授权状态、密钥指纹和本地路径。

</details>

## 人 / Agent 操作占比

```text
操作顺序
1. [Agent] 生成独立密钥并报告指纹
2. [用户] 核对指纹并完成首次授权（可按需临时使用 root）
3. [Agent] 创建无 sudo、无提权权限的专属账号
4. [Agent] 安装公钥并验证专属账号登录
——正常工作，未来该agent可直接用此身份登陆，随时接管——

若用户选择撤销该agent访问权限
1. [用户] 按“指纹 + 账号”暂停授权；需要时可恢复
2. [用户] 确认不再需要后彻底撤销服务器公钥
3. [用户] 复制彻底删除提示词，在工作机卸载内存缓存并删除磁盘私钥
```

## 提供给外部 Agent 的完整提示词

接管提示词是面向所有舟SSH用户的通用模板，不读取开发者或当前设备上的名称、账号、指纹和密钥路径。每位用户都可以自行指定显示名称、公钥标签和专属账号，也可以让 Agent 为本次接管随机生成。提示词不包含服务器地址、密码、公钥或私钥正文。

<details>
<summary>展开完整提示词</summary>

```text
你将从当前工作机通过 SSH 接管一台由“舟SSH”管理的服务器。

1. 显示名称、公钥标签和专属账号可以由当前用户指定；用户选择随机或没有指定时，为本次接管生成一组全新的 8 位小写字母数字后缀。公钥标签和专属账号使用 `qssh-agent-<后缀>`，账号必须符合 Linux 用户名格式。生成后向当前用户报告最终名称，不得复用其他用户的数据。
2. 为本次接管生成独立 Ed25519 密钥，使用上述公钥标签作为注释。私钥只能留在当前工作机；不得复用默认密钥、输出私钥、启用 Agent Forwarding 或绕过主机指纹检查。
3. 只向用户报告显示名称、专属账号、私钥路径、公钥路径、完整 SHA-256 指纹、标签和完整公钥一行。
4. 用户可按需临时授权 root；接管结束时是否保留同指纹的 root 授权由用户在舟SSH中决定，不强制取消。
5. 必须创建或确认最终确定的专属账号。它不得加入 sudo、wheel、admin 等管理组，不得写入 sudoers，不得拥有 sudo、su、doas、pkexec 等提权能力，也不设置可用密码。
6. 将同一公钥安装到专属账号，设置正确的所有者以及目录 700、文件 600 权限，并从全新 SSH 会话验证可以登录。
7. 发现目标服务器不符、需要删除数据、修改 SSH 全局配置、开放额外端口或需要任何提权时，立即停止并说明原因。
8. 完成后报告专属账号、公钥指纹、各账号当前授权、修改内容和遗留风险。提醒用户可以按“指纹 + 账号”分别撤销、恢复或彻底撤销。

现在先生成独立密钥，输出允许报告的信息和已替换完整公钥的安装命令，然后等待用户确认。
```

</details>

## 数据与隐私

密钥管理卡不要求用户登记 Agent 名称、专属账号或工作机私钥路径。它只读取当前服务器扫描返回的脱敏密钥状态，并按 SHA-256 指纹临时归类展示。

密钥管理卡不会保存 SSH 私钥、公钥正文、服务器密码、Agent 对话或思考过程。远端扫描返回前端的密钥信息只有用户名、算法、标签、指纹和撤销状态。

## 扫描范围与服务器资源

该功能不会在服务器上安装守护程序，也不会创建远端常驻监管进程。面板只通过已有 SSH 连接短暂执行只读命令：

- 读取当前权限能够访问的 `authorized_keys`
- 使用 `who` 查看带终端的会话
- 使用 `ps` 识别无终端 SSH 会话
- 在 root 权限下读取近 24 小时的 SSH 成功认证日志，最多取 500 条用于身份归属

自动扫描间隔为 30 秒，软件窗口不可见时暂停。扫描产生的 shell、`who`、`ps`、`journalctl`、`grep` 和 `tail` 都会在命令结束后退出，不持续占用远端内存，只产生轻微、间歇性的 CPU 与日志读取开销。

## 权限边界与限制

- root 连接可以扫描 `/root/.ssh/authorized_keys` 和 `/home/*/.ssh/authorized_keys`；普通用户通常只能检查自己的授权文件。
- 撤销、恢复和彻底撤销都按完整指纹精确处理，不按公钥注释模糊匹配。
- 普通用户只能管理自己的公钥；root 连接可以跨账号管理。
- 权限详情也是按需只读检查：root 连接可以检查其他账号，普通连接只能检查当前登录账号；查看权限不会执行列出的 sudo 命令。
- 撤销公钥只阻止后续新登录；已经建立的 SSH 会话仍需单独终止。
- 工作机私钥彻底删除与服务器公钥撤销是两件事。删除工作机私钥后，即使恢复服务器公钥，也无法再使用原私钥登录。
- 防火墙、服务管理、AI 命令执行等功能会修改远端状态，请在操作前检查目标服务器和命令内容。

## 例行修复
安全加固 + 性能优化 + 死代码
安全:
- SSH 主机密钥改用 TOFU 校验（ssh/hostkey.go），替换 InsecureIgnoreHostKey，防中间人
- 云端客户端启用 TOFU 证书固定（cloud/client），替换 InsecureSkipVerify
- 云端服务器 sync-pull/push/heartbeat 增加令牌认证，堵住匿名拖库
- WebSocket CheckOrigin 仅放行无 Origin 的原生客户端，防 CSWSH
- 修复命令注入：新增 ssh/shellsafe.go，firewall/guardian/monitor/sftp 全部
  改用白名单校验 + shellQuote 单引号转义
- 凭据/密钥/配置文件权限收紧为 0600，目录 0700，新增 apppaths.WriteSecure
  （对旧文件补 chmod）；云端 key.pem/sync.json/config.json 同步收紧
- 云端 nonce 改用 crypto/rand，去掉时间派生的可预测实现

性能:
- 主循环 500ms 广播增加哈希变更检测，无变化不推送
- 终端输出轮询改自适应退避（20→100ms）+ 32KB 缓冲
- 修复 AppLayout/TopBookmarkBar 的 setInterval 泄漏（onUnmounted 清理）
- vite 拆分 xterm/highlight/markdown chunk，主包 2.9MB→1.6MB

死代码清理:
- 删除 time 事件链（emitter + RegisterEvent + HelloWorld.vue 消费者）
- 删除未使用的 main.readFromShell、零启.svg
- 移除未引用依赖：monaco-editor、@vueuse/core、jszip、splitpanes、
  vue-splitpane、xterm-addon-serialize

## 系统要求

- macOS 11.0 或更高版本
- Apple Silicon（arm64）或 Intel（amd64）Mac

## 获取与安装

可以从本仓库的 [Releases](https://github.com/qxryz/qsshformac/releases) 页面获取构建产物，也可以按照下方步骤从源码构建。

当前构建产物使用 ad-hoc 签名。如果首次打开被 Gatekeeper 拦截，请确认文件来源可信后执行：

```bash
xattr -cr /path/to/pzssh.app
```

## 从源码构建

需要 Go 1.25+、Node.js 18+ 和 Xcode Command Line Tools。

```bash
# 安装 Wails v3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# 安装前端依赖
cd frontend
npm install
cd ..

# 开发模式
wails3 dev

# 构建当前架构的 .app，输出到 bin/pzssh.app
wails3 task darwin:package

# 分别构建 arm64 与 amd64 便携压缩包
wails3 task darwin:package:portable

# 构建通用二进制 .app
wails3 task darwin:package:universal
```

## 技术栈

- 后端：Go、Wails v3
- 前端：Vue 3、Pinia、xterm.js、dockview-vue
- SSH：`golang.org/x/crypto/ssh`

## 上游项目与许可证

本项目基于 [nanxiangxi/qssh](https://github.com/nanxiangxi/qssh) 移植和修改。

- 原作者：[nanxiangxi](https://github.com/nanxiangxi)
- macOS 移植仓库：[qxryz/qsshformac](https://github.com/qxryz/qsshformac)
- 用途：非商业开源项目

本项目沿用原项目的 **CC BY-NC-SA 4.0** 许可证。使用、修改或分发前，请阅读 [CC BY-NC-SA 4.0 许可说明](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)。
