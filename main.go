package main

import (
	"changeme/ai"
	"changeme/ssh"
	"crypto/sha256"
	"embed"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// hashJSON 返回任意值 JSON 序列化后的短哈希，用于状态变化检测。
func hashJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// Wails 使用 Go 的 `embed` 包将前端文件嵌入到二进制文件中。
// frontend/dist 文件夹中的所有文件将被嵌入到二进制文件中，
// 并提供给前端使用。
// 更多信息请参见 https://pkg.go.dev/embed

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

// 版本信息
const (
	AppVersion = "0.3.2.1"
	AppName    = "舟SSH"
)

func init() {
	// ssh:connections-updated 和 ssh:group-updated 不注册具体类型，使用 map[string]interface{}
}
func main() {

	// 通过提供必要的选项创建一个新的 Wails 应用程序。
	// 'Name' 和 'Description' 是应用程序元数据。
	// 'Assets' 配置资产服务器，'FS' 变量指向前端文件。
	// 'Services' 是 Go 结构体实例列表。前端可以访问这些实例的方法。
	// 'Mac' 选项在 macOS 上运行时定制应用程序。

	// 创建应用实例
	app := application.New(application.Options{
		Name:        AppName,
		Description: fmt.Sprintf("一个中文的SSH工具，便携，简单，开源 (v%s)", AppVersion),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false, // 托盘应用：关闭窗口不退出
		},
	})

	// 创建SSH服务并设置窗口管理器和app实例
	sshService := ssh.NewSSHService()

	// 创建AI服务
	aiService := ai.NewAIService()

	// 创建配置服务
	configService := ssh.NewConfigService()
	configService.SetApp(app)

	// 创建窗口管理器（分组关闭回调稍后设置）
	windowManager := ssh.NewWindowManager(app, nil)
	windowManager.SetConfigService(configService)
	sshService.SetWindowManager(windowManager)
	sshService.SetApp(app)

	// 设置AI服务
	aiService.SetApp(app)
	ai.InitDeps(aiService, sshService, sshService)

	// 启动健康检查
	sshService.StartHealthCheck()

	// 创建端口转发服务
	portForwardService := ssh.NewPortForwardService(sshService)

	// 创建防火墙服务
	firewallService := ssh.NewFirewallService(sshService)

	// 创建进程守护服务
	guardianService := ssh.NewProcessGuardianService(sshService)

	// 创建云端服务
	cloudService := ssh.NewCloudService(configService)
	cloudService.SetApp(app)

	// 注册服务到应用
	app.RegisterService(application.NewService(ssh.NewGreetService(AppVersion)))
	app.RegisterService(application.NewService(sshService))
	app.RegisterService(application.NewService(aiService))
	app.RegisterService(application.NewService(configService))
	app.RegisterService(application.NewService(portForwardService))
	app.RegisterService(application.NewService(firewallService))
	app.RegisterService(application.NewService(guardianService))
	app.RegisterService(application.NewService(cloudService))

	// 使用必要的选项创建一个新窗口。
	// 在创建窗口前获取已保存的位置，直接在 WebviewWindowOptions 中设置初始位置
	// 这样窗口创建时就出现在正确位置，避免先显示在默认位置再跳转
	savedPos := windowManager.GetSavedMainWindowPosition()

	windowOpts := application.WebviewWindowOptions{
		Name:             "main",
		Title:            fmt.Sprintf("舟SSH - SSH工具 (v%s)", AppVersion),
		URL:              "/",
		DisableResize:    false,
		Frameless:        true,
		BackgroundColour: application.NewRGB(255, 255, 255),
	}

	if savedPos != nil {
		// 使用保存的位置和大小
		windowOpts.InitialPosition = application.WindowXY
		windowOpts.X = savedPos.X
		windowOpts.Y = savedPos.Y
		windowOpts.Width = savedPos.Width
		windowOpts.Height = savedPos.Height
		fmt.Printf("[Main] 使用保存的窗口位置: (%d,%d %dx%d)\n", savedPos.X, savedPos.Y, savedPos.Width, savedPos.Height)
	} else {
		// 没有保存位置，使用默认大小居中
		w, h := calculateWindowSize(app)
		windowOpts.Width = w
		windowOpts.Height = h
		// InitialPosition 默认为 WindowCentered
		fmt.Printf("[Main] 使用默认窗口大小: %dx%d\n", w, h)
	}

	mainWindow := app.Window.NewWithOptions(windowOpts)

	// 设置分组关闭回调（所有 SSH 窗口关闭后自动显示主窗口）
	windowManager.SetOnGroupClose(func(groupID string) {
		fmt.Printf("[Main] 🗑️ 窗口销毁，关闭分组: %s\n", groupID)
		if err := sshService.CloseGroup(groupID); err != nil {
			fmt.Printf("[Main] ⚠️ 关闭分组失败: %v\n", err)
		}
		// 检查是否启用了自动显示首页
		cfg := configService.GetConfig()
		if windowManager.GetWindowCount() == 0 && cfg.UI.AutoShowHome {
			fmt.Println("[Main] 所有 SSH 窗口已关闭，显示主窗口")
			mainWindow.Show()
			mainWindow.Focus()
		}
	})

	// 定时保存主窗口位置（每3秒检查一次）
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			windowManager.SaveMainWindowPositionIfChanged(mainWindow)
		}
	}()

	// 监听主窗口关闭事件，保存位置
	mainWindow.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		fmt.Println("[Main] 主窗口关闭，保存位置")
		windowManager.SaveMainWindowPosition(mainWindow)
	})

	// ========== 系统托盘 ==========
	systray := app.SystemTray.New()
	systray.SetIcon(trayIcon)
	systray.SetLabel(AppName)

	// 托盘菜单
	trayMenu := app.NewMenu()
	trayMenu.Add("显示主窗口").OnClick(func(ctx *application.Context) {
		mainWindow.Show()
		mainWindow.Focus()
	})
	trayMenu.Add("显示全部窗口").OnClick(func(ctx *application.Context) {
		mainWindow.Show()
		mainWindow.Focus()
		windowManager.ShowAllWindows()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("隐藏全部窗口").OnClick(func(ctx *application.Context) {
		windowManager.HideAllWindows()
		windowManager.SaveMainWindowPosition(mainWindow)
		mainWindow.Hide()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出").OnClick(func(ctx *application.Context) {
		mainWindow.Show() // 退出前显示窗口，确保正常关闭
		windowManager.ShowAllWindows()
		app.Quit()
	})
	systray.SetMenu(trayMenu)

	// 左键点击托盘图标切换主窗口显示/隐藏
	systray.OnClick(func() {
		if mainWindow.IsVisible() {
			windowManager.SaveMainWindowPosition(mainWindow)
			mainWindow.Hide()
		} else {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})

	// 监听前端请求隐藏主窗口的事件（SSH连接成功后自动隐藏主窗口，不影响SSH窗口）
	app.Event.On("ssh:tray-hide", func(event *application.CustomEvent) {
		fmt.Println("[Main] 收到托盘隐藏请求，隐藏主窗口")
		windowManager.SaveMainWindowPosition(mainWindow)
		mainWindow.Hide()
	})

	fmt.Println("[Main] 系统托盘已初始化")

	// 创建一个 goroutine，每500ms广播SSH分组状态。
	// 仅在状态变化时才发送事件，避免无变化时反复推送整份连接列表，
	// 减少前端反序列化与重渲染压力。
	go func() {
		lastGroupHash := make(map[string]string)
		lastConnHash := ""
		for {
			time.Sleep(500 * time.Millisecond)

			// 获取所有分组
			groups := sshService.GetAllGroups()
			seen := make(map[string]bool, len(groups))

			// 为每个分组发送状态（仅在变化时）
			for _, group := range groups {
				seen[group.ID] = true
				connInfos := sshService.GetGroupConnectionInfos(group.ID)
				h := hashJSON(connInfos)
				if lastGroupHash[group.ID] == h {
					continue // 无变化，跳过
				}
				lastGroupHash[group.ID] = h
				app.Event.Emit("ssh:group-updated", map[string]interface{}{
					"groupID":     group.ID,
					"action":      "state-sync",
					"connections": connInfos,
				})
			}
			// 清理已消失分组的缓存
			for gid := range lastGroupHash {
				if !seen[gid] {
					delete(lastGroupHash, gid)
				}
			}

			// 广播全局连接状态（用于首页侧边栏），仅在变化时
			allConnections := sshService.GetAllConnections()
			if allConnections == nil {
				allConnections = []*ssh.ConnectionInfo{}
			}
			h := hashJSON(allConnections)
			if h != lastConnHash {
				lastConnHash = h
				app.Event.Emit("ssh:connections-updated", map[string]interface{}{
					"connections": allConnections,
					"timestamp":   time.Now().UnixMilli(),
				})
			}
		}
	}()

	// 创建一个 goroutine，定期读取所有 Shell session 的输出并发送到前端。
	// 采用自适应轮询：有输出时保持 20ms 高频，空闲时逐步退避到 100ms，
	// 降低无终端活动时的 CPU 占用，同时不影响活跃终端的响应速度。
	go func() {
		buf := make([]byte, 32*1024) // 32KB 缓冲，减少高吞吐时的分片与事件数
		const minInterval = 20 * time.Millisecond
		const maxInterval = 100 * time.Millisecond
		interval := minInterval
		for {
			time.Sleep(interval)

			gotData := false
			connIDs := sshService.GetConnectionList()
			for _, connID := range connIDs {
				sessionIDs := sshService.GetSessionIDs(connID)
				for _, sessionID := range sessionIDs {
					if !sshService.IsShellSessionActive(connID, sessionID) {
						continue
					}
					n, err := sshService.ReadFromShellSession(connID, sessionID, buf)
					if err != nil {
						continue
					}
					if n > 0 {
						gotData = true
						app.Event.Emit("ssh:terminal-output", map[string]interface{}{
							"connID":    connID,
							"sessionID": sessionID,
							"data":      string(buf[:n]),
						})
					}
				}
			}

			// 自适应：有数据则回到高频，空闲则退避
			if gotData {
				interval = minInterval
			} else if interval < maxInterval {
				interval += 20 * time.Millisecond
			}
		}
	}()

	// 运行应用程序。这将阻塞直到应用程序退出。
	appErr := app.Run()

	// 如果运行应用程序时发生错误，记录错误并退出。
	if appErr != nil {
		log.Fatal(appErr)
	}
}

// calculateWindowSize 根据屏幕大小计算合适的窗口尺寸
func calculateWindowSize(app *application.App) (int, int) {
	type size struct {
		width  int
		height int
	}
	sizes := []size{
		{1920, 1080},
		{1600, 1000},
		{1400, 900},
		{1200, 800},
	}

	primary := app.Screen.GetPrimary()
	if primary == nil {
		return 1400, 900
	}

	screenW := primary.Size.Width
	screenH := primary.Size.Height

	maxW := int(float64(screenW) * 0.85)
	maxH := int(float64(screenH) * 0.85)

	fmt.Printf("[Main] 屏幕大小: %dx%d, 最大窗口: %dx%d\n", screenW, screenH, maxW, maxH)

	for _, s := range sizes {
		if s.width <= maxW && s.height <= maxH {
			return s.width, s.height
		}
	}

	return 1200, 800
}
