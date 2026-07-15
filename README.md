# 舟SSH（qssh 0.3.2.1 for mac）

舟SSH 是一款面向 macOS 的中文 SSH 客户端，提供终端、SFTP、端口转发、服务器管理、AI 辅助操作和按指纹组织的 SSH 密钥管理。

本项目移植自 [nanxiangxi/qssh](https://github.com/nanxiangxi/qssh)，在保留原有能力的基础上完成 macOS 适配，并将应用命名为「舟SSH」、内置 AI 助手命名为「舟舟」。

## 功能概览

- 多标签 SSH 终端，支持经典 xterm 和结构化终端
- SFTP 文件管理：上传、下载、预览和在线编辑
- 连接保存、快速连接、分组与云端同步
- 本地与远程端口转发
- 批量命令、操作日志、系统监控和进程守护
- iptables、firewalld、ufw 防火墙管理
- 支持 OpenAI 兼容 API 的内置 AI 助手
- SSH 密钥管理：按指纹归类账号、在线识别、权限审计、撤销、恢复和彻底撤销

## macOS 适配

- 标准 `.app` 应用包
- 数据目录：`~/Library/Application Support/qssh`
- 支持 `⌘+C/V`、`⌘+←/→`、`⌘+Shift+S/U/D` 等 macOS 快捷键
- 终端中的 `Ctrl+C` 保持 Unix 中断语义
- 终端和编辑器优先使用 SF Mono、Menlo、Monaco
- 菜单栏图标与多窗口管理
- 云端同步按实际 macOS 平台注册设备

## SSH 密钥管理（0.3.2.1）

右侧工具栏的「SSH 密钥管理」是一张按完整 SHA-256 公钥指纹组织的管理卡。这里的 Agent 指在另一台工作机上使用独立 SSH 私钥连接服务器的 Codex 或其他自动化工具，不是舟SSH内置 AI 助手。

当前实现面向 OpenSSH Linux 服务器。root 连接可以扫描和管理 `/root/.ssh/authorized_keys` 与 `/home/*/.ssh/authorized_keys`；普通连接通常只能查看和管理当前登录账号。

### 指纹与账号

- 完整 SHA-256 指纹是主键，公钥注释只作为标签展示。
- 同一指纹可以登录多个账号，每个“指纹 + 账号”授权独立显示和操作。
- 面板不登记 Agent 名称、私钥路径或专属账号，也不建立本地 Agent 档案。
- 公钥标签、显示名称和账号名称可以由使用者指定，也可以在接管时随机生成。

### 在线识别

- 面板结合 `who`、`sshd` 进程和 SSH 成功认证日志识别会话。
- 只有认证日志能把账号和指纹对应起来时，才显示为该指纹确认上线。
- 在线信息包括账号、会话数、终端、登录时间和脱敏来源。
- 扫描只说明登录状态，不能观察 Agent 的思考过程或完整操作内容。
- 面板打开且窗口可见时每 15 秒自动刷新，也可以手动刷新。

### 只读权限审计

每个账号的「权限」按钮按需读取用户组和 `sudo -n -l` 输出：

- 未检查：非 root 账号只显示“权限待检查”，不预先当作普通用户。
- 低权限账号：没有 sudo 规则。
- 受限运维账号：只有明确列出的 sudo 命令。
- 管理员账号：拥有完整 sudo 或账号本身是 root。

权限审计不会执行列出的 sudo 命令，也不提供增加权限功能。名称为 `admin` 不代表权限高低，最终分类以实际用户组和 sudo 规则为准。

### 撤销、恢复与彻底撤销

- 撤销：把对应公钥从 `authorized_keys` 移入同账号的 `authorized_keys.qssh-revoked`，阻止新登录并保留恢复能力。
- 恢复：把公钥从可恢复区放回 `authorized_keys`。
- 彻底撤销：同时从当前授权和可恢复区删除对应公钥；之后只能重新安装公钥。
- 所有操作按完整指纹匹配，并在改写前检查文件、哈希和符号链接状态。
- 改写前保留 `.qssh.bak` 备份，避免并发修改时静默覆盖。

撤销公钥只阻止后续新登录，已经建立的 SSH 会话不会被自动终止。

### 两个复制按钮

面板保留两份通用提示词：

1. Agent 接管提示词：要求创建专属普通账号，不加入 `sudo`、`wheel`、`admin` 等管理组，不写入 sudoers，不授予 `sudo`、`su`、`doas`、`pkexec` 等提权能力。是否保留同指纹的 root 授权由用户决定。
2. 工作机私钥彻底删除提示词：先由当前用户确认真实指纹和私钥路径，再只从 `ssh-agent` 卸载指定密钥，删除磁盘上的私钥与 `.pub` 文件，并分别验证内存和磁盘状态。

提示词是面向所有舟SSH用户的通用模板，不会注入开发者本机的用户名、服务器、指纹或路径。应用内按钮复制出的内容是当前版本的唯一标准模板，README 不再复制一份容易过期的命令副本。

`ssh-agent` 是缓存已加载私钥的后台进程，不是文件夹；`SSH_AUTH_SOCK` 指向它的通信 socket。私钥文件通常位于工作机的 `~/.ssh/`，但删除前必须让实际使用者确认真实路径。

## 数据与隐私

密钥管理扫描返回前端的内容仅包括：

- 账号名、算法、公钥标签和 SHA-256 指纹
- 当前授权或已撤销状态
- 指纹确认的 SSH 会话元数据
- 用户主动点击后读取的用户组和 sudo 规则

不会返回或保存 SSH 私钥、公钥正文、服务器密码、Agent 对话或思考过程。

## 远端资源与限制

密钥管理不会安装守护程序，也不会创建常驻进程。扫描命令读取授权文件、`who`、`ps` 和可用的 SSH 认证日志后立即退出。完整跨账号扫描、认证日志读取和跨账号操作通常需要 root 连接。

工作机私钥删除和服务器公钥撤销是两件事。删除工作机私钥后，即使恢复服务器公钥，也无法再用原私钥登录。SSD、APFS 或日志型文件系统上的普通文件删除不能保证物理介质级不可恢复；高敏感场景应配合整盘加密和密钥轮换。

## 系统要求与安装

- macOS 11.0 或更高版本
- 0.3.2.1 Release 提供 Apple Silicon（arm64）压缩包
- 源码构建任务也支持 amd64 和 universal 应用包

从 [Releases](https://github.com/qxryz/qsshformac/releases) 下载构建产物。当前发布包使用 ad-hoc 签名；如果首次打开被 Gatekeeper 拦截，请确认文件来源可信后执行：

```bash
xattr -cr /path/to/pzssh.app
```

## 从源码构建

需要 Go 1.25+、Node.js 18+、Xcode Command Line Tools 和 Wails v3 CLI。

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

cd frontend
npm install
cd ..

# 开发模式
wails3 dev -config ./build/config.yml

# 当前架构生产应用包：bin/pzssh.app
wails3 task package

# arm64 与 amd64 便携压缩包
wails3 task darwin:package:portable

# universal 应用包
wails3 task darwin:package:universal

# 构建、打开并确认应用进程启动
./script/build_and_run.sh --verify
```

## 技术栈

- 后端：Go 1.25、Wails v3
- 前端：Vue 3、Pinia、Vite、xterm.js、Dockview、CodeMirror
- SSH：`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`

## 版本记录

版本变更见 [CHANGELOG.md](./CHANGELOG.md)，当前代码结构见 [CODE_WIKI.md](./CODE_WIKI.md)。

## 上游项目

本项目基于 [nanxiangxi/qssh](https://github.com/nanxiangxi/qssh) 移植和修改。
