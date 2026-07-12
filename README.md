# 舟SSH（qssh 0.3.2 for mac）

一个中文的 SSH 远程连接工具，开源、好看。本仓库是 [nanxiangxi/qssh](https://github.com/nanxiangxi/qssh) 的 **macOS (Apple Silicon) 移植版**，并更名为舟SSH，内置 AI 助手舟舟。

## 关于本移植版

原版是 Windows 便携式应用，本仓库将其完整功能移植到 macOS：

- ✅ 原生 arm64 (Apple Silicon) 构建，打包为标准 `.app`
- ✅ 数据目录改用 macOS 规范位置：`~/Library/Application Support/qssh`（原版存放在 exe 同级 `data/` 目录，在 .app 内不可写）
- ✅ 快捷键适配：`⌘+C/V` 复制粘贴、`⌘+←/→` 切换标签、`⌘+Shift+S/U/D` 等（Ctrl 组合键仍然可用；终端内 `Ctrl+C` 保持 Unix 中断语义）
- ✅ 终端/编辑器字体优先使用 SF Mono / Menlo / Monaco
- ✅ 系统托盘 → macOS 菜单栏图标
- ✅ 云端同步设备注册按实际平台上报

## 功能（与原版一致）

- 中文界面，开箱即用
- 暗色主题，好看优先
- 多标签终端管理（经典 xterm 模式 + 结构化模式）
- SFTP 文件管理（上传/下载/在线编辑）
- 内置 AI 助手（OpenAI 兼容 API，可执行远程命令）
- 端口转发（本地/远程）
- 防火墙管理（iptables / firewalld / ufw，作用于远程服务器）
- 进程守护（systemd 服务管理，作用于远程服务器）
- 批量命令执行
- 连接管理（保存/编辑/快速连接/分组）
- 云端同步

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

- macOS 11.0+（Apple Silicon）

## 构建

需要：Go 1.25+、Node.js 18+、Xcode Command Line Tools

```bash
# 安装 wails3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# 安装前端依赖
cd frontend && npm install && cd ..

# 开发模式
wails3 dev

# 构建 .app（输出到 bin/pzssh.app）
wails3 task darwin:package

# 构建 arm64 + amd64 便携 zip
wails3 task darwin:package:portable
```

构建产物为 ad-hoc 签名。首次打开如被 Gatekeeper 拦截，可执行：

```bash
xattr -cr bin/pzssh.app
```

## 技术栈

- 后端：Go + Wails v3
- 前端：Vue 3 + xterm.js + dockview-vue
- SSH：golang.org/x/crypto/ssh

## 许可证

与原项目一致，基于 **CC BY-NC-SA 4.0** 协议开源。

- 原作者：[nanxiangxi](https://github.com/nanxiangxi)（原项目 [qssh](https://github.com/nanxiangxi/qssh)）
- 本仓库为非商业用途的移植修改版，遵循相同协议开源

详情请参阅 [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)
