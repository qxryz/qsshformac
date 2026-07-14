# 舟SSH（qssh for mac）

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
- 外部 Agent SSH 密钥监管：登记密钥、识别登录、撤销权限

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

舟SSH右侧工具栏中的“外部 Agent 监管”面板可以：

- 登记 Agent 名称、公钥标签、SHA-256 指纹和最终专用用户名
- 扫描服务器中尚未登记的授权密钥并一键纳管
- 区分用户自己的 root 登录、已登记 Agent 的临时 root 和其他密钥登录
- 识别带终端会话以及 `sshd: <user>@notty` 形式的无终端 Agent 会话
- 显示等待接入、临时 root、迁移未完成、权限已收敛和当前在线等状态
- 按完整公钥指纹从服务器撤销授权，并创建 `authorized_keys.qssh.bak` 备份
- 生成需要在工作机执行的私钥卸载与清理命令
- 复制结构化接管提示词，让外部 Agent 遵守统一的权限生命周期

### 登录归属

面板不会只看用户名。它会结合已登记密钥指纹、当前 `sshd` 会话和 SSH 认证日志判断：

- **用户 root 登录**：认证日志显示为密码或交互式认证。
- **Agent 临时 root**：公钥指纹与已登记 Agent 匹配。
- **其他密钥登录**：存在公钥指纹，但尚未匹配已登记 Agent。
- **身份未确认**：权限不足、日志缺失或认证证据不足。

公钥注释中的 `admin`、`laptop` 等文字只是标签，不代表密钥一定属于用户或 Agent。扫描结果必须由用户确认；一键纳管只创建本地监管记录，不会自动修改服务器权限。

## 推荐的安全接管流程

1. 外部 Agent 在工作机生成一把独立 Ed25519 密钥。
2. 私钥只保存在该工作机；Agent只报告公钥、标签、指纹和私钥保存路径。
3. 用户核对信息，并亲自把公钥临时安装到服务器的 root 账号。
4. Agent使用临时 root 安装服务、完成必要初始化并创建专用用户。
5. Agent将同一公钥迁移到专用用户，并验证专用用户可以登录。
6. 用户确认迁移成功后，从 root 的 `authorized_keys` 撤销该公钥。
7. 后续任务只使用专用用户；再次使用 root 必须重新说明原因并取得批准。
8. 不再需要接管时，先撤销服务器公钥，再在工作机卸载并删除私钥材料。

root 在这里是一次性的引导权限，不是 Agent 的长期运行账号。

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
2. [用户] 核对指纹并临时安装 root 公钥
3. [Agent] 使用临时 root 安装服务
4. [Agent] 创建专用用户并迁移公钥
5. [Agent] 验证专用用户登录
——正常工作，未来该agent可直接用此身份登陆，随时接管——

若用户选择撤销该agent访问权限
1. [用户] 确认并撤销 root 公钥（一键）
2. [用户] 在「工作机」核对并清理私钥
```

## 提供给外部 Agent 的完整提示词

登记 Agent 后，面板会自动把 `<KEY_LABEL>` 和 `<SERVICE_USER>` 替换为当前记录的管理标签和专用用户名。提示词不包含服务器地址、密码、公钥或私钥正文。

<details>
<summary>展开完整提示词</summary>

```text
你将从一台外部主力机通过 SSH 接管一台由“舟SSH”监管的服务器。你的任务是把接管流程完整做完，并把用户必须亲自执行的步骤写成可直接复制的命令。

固定身份与安全边界
- 为本次接管生成独立 Ed25519 密钥，管理标签必须是：<KEY_LABEL>
- 最终长期使用的专用用户名必须是：<SERVICE_USER>
- 私钥只能保存在当前工作机，不得输出、上传、粘贴进聊天或写入远端
- 远端只安装公钥；不得索取、记录或保存服务器密码
- 不得启用 SSH Agent Forwarding，不得复用个人默认密钥，不得使用 StrictHostKeyChecking=no
- root 仅用于首次部署和权限迁移；不得作为 Agent 的长期工作账号

阶段一：在工作机准备独立密钥
1. 选择清晰的独立私钥路径，先检查该路径是否已存在；如果存在，停止并更换路径，绝不覆盖旧密钥。
2. 使用 ssh-keygen -t ed25519 生成密钥，注释必须是 <KEY_LABEL>。
3. 只向用户报告：私钥路径、公钥路径、完整 SHA-256 指纹、管理标签和完整公钥一行。绝不显示私钥正文。

阶段二：明确告诉用户如何临时安装 root 公钥
1. 告诉用户先使用自己的认证方式登录服务器，例如：ssh root@<SERVER_HOST>。
2. 将下面命令中的 <PUBLIC_KEY_LINE> 替换为刚生成的完整公钥一行，然后把替换后的完整命令块交给用户。不得保留占位符，不得让用户自行拼接：

install -d -m 700 /root/.ssh
KEY_LINE='<PUBLIC_KEY_LINE>'
touch /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
grep -qxF "$KEY_LINE" /root/.ssh/authorized_keys || printf '%s\n' "$KEY_LINE" >> /root/.ssh/authorized_keys
ssh-keygen -lf /root/.ssh/authorized_keys

3. 解释最后一条命令用于让用户核对已安装密钥的 SHA-256 指纹。
4. 在这里暂停，只等待用户回复“已安装”。这是正常流程中唯一必需的用户交接点；在用户确认前不得尝试连接。

阶段三：Agent 使用临时 root 完成部署
1. 用户确认后，使用独立密钥和 IdentitiesOnly=yes 连接 root；首次出现服务器主机指纹时，要求用户核对，不得绕过主机身份检查。
2. 连接后先用 id、hostname 等只读命令确认当前服务器和账号，再开始部署。
3. 一次性说明本阶段准备安装的软件、修改的文件、开放的端口、创建的服务和回滚方法，然后连续完成已约定的部署任务；无需对每条普通命令重复索取确认。
4. 每条命令执行后记录退出状态和实际变化。如果发现目标服务器不符、需要删除用户数据、修改 SSH 全局配置、扩大防火墙暴露面或执行计划外高风险操作，立即停止并单独询问用户。

阶段四：创建专用用户并迁移密钥
1. 创建或确认专用用户 <SERVICE_USER>，默认不给密码登录能力，不授予完整免密 sudo。
2. 只授予任务真正需要的最小目录权限、服务权限或精确 sudo 命令；说明每项权限的用途。
3. 将同一公钥写入 <SERVICE_USER> 的 authorized_keys，正确设置主目录、.ssh 目录和文件的所有者及 700/600 权限；操作必须可重复执行且不能产生重复公钥。
4. 保留当前 root 会话，同时从一个新的独立 SSH 会话使用该密钥登录 <SERVICE_USER>，执行 id 和任务所需的最小验证，证明专用账号确实可用。

阶段五：验证成功后收回 root
1. 只有专用用户的新会话验证成功后，才能处理 root 授权；验证失败时保留 root 公钥并先修复问题。
2. 先备份 /root/.ssh/authorized_keys，再按完整 SHA-256 指纹精确删除本 Agent 的 root 公钥，不得只按模糊文字删除，也不得影响其他密钥。
3. 确认 root 的 authorized_keys 中已不存在该指纹，确认专用用户仍可登录，然后退出临时 root 会话。
4. 后续所有正常任务只使用 <SERVICE_USER>。若以后确实需要 root，先说明原因和范围，再让用户重新授权。

阶段六：交付与将来撤销
1. 完成后报告：服务器、专用用户名、公钥指纹、密钥标签、root 授权是否已撤销、专用用户权限、服务状态、修改内容、回滚方法和遗留风险。
2. 向用户说明：只要还需要该 Agent 接管服务器，就必须保留工作机私钥；不要在正常部署完成后删除它。
3. 同时给出“彻底撤销 Agent”时的两段操作：先在服务器删除专用用户 authorized_keys 中的对应指纹，再在工作机用 ssh-add -d 卸载并用 rm -i 删除私钥和 .pub 文件。必须带入真实用户名、指纹和已确认的私钥路径，不得留下含糊占位符。

现在先执行阶段一，然后输出密钥信息和阶段二中已经替换好公钥的用户命令块，并停下来等待用户回复“已安装”。
```

</details>

## 数据与隐私

外部 Agent 监管记录只在本地保存以下元数据：

- Agent 名称
- 公钥管理标签
- 最终专用用户名
- 已纳管公钥的 SHA-256 指纹
- 可选的工作机私钥路径，仅用于生成清理命令
- 由连接信息生成的不可逆作用域 ID，用于稳定关联记录

监管记录不会保存 SSH 私钥、公钥正文、服务器密码、服务器地址明文、Agent 对话或思考过程。远端扫描返回前端的密钥信息只有用户名、算法、标签和指纹；来源 IP 在界面中会被遮罩。

## 扫描范围与服务器资源

该功能不会在服务器上安装守护程序，也不会创建远端常驻监管进程。面板只通过已有 SSH 连接短暂执行只读命令：

- 读取当前权限能够访问的 `authorized_keys`
- 使用 `who` 查看带终端的会话
- 使用 `ps` 识别无终端 SSH 会话
- 在 root 权限下读取近 24 小时的 SSH 成功认证日志，最多取 500 条用于身份归属

自动扫描间隔为 30 秒，软件窗口不可见时暂停。扫描产生的 shell、`who`、`ps`、`journalctl`、`grep` 和 `tail` 都会在命令结束后退出，不持续占用远端内存，只产生轻微、间歇性的 CPU 与日志读取开销。

## 权限边界与限制

- root 连接可以扫描 `/root/.ssh/authorized_keys` 和 `/home/*/.ssh/authorized_keys`；普通用户通常只能检查自己的授权文件。
- “当前在线”表示发现身份匹配的 SSH 会话，不代表能够观察 Agent 的思考过程或完整工作内容。
- 身份归属依赖服务器保留可读的 SSH 认证日志；日志缺失或权限不足时会显示“身份未确认”。
- 一键纳管不会自动判断密钥的真实所有者，也不会修改服务器权限。
- 移除本地监管记录不会删除服务器公钥。
- 撤销公钥只阻止后续新登录；已经建立的 SSH 会话可能仍需单独终止。
- 防火墙、服务管理、AI 命令执行等功能会修改远端状态，请在操作前检查目标服务器和命令内容。

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
