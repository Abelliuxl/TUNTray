package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/ncruces/zenity"
)

// --- Constants ---
const (
	tunAlias       = "wintun"
	tunIP          = "192.168.123.1"
	tunMask        = "255.255.255.0"
	tunDNS         = "8.8.8.8"
	configFile     = "config.json"
	oldProxiesFile = "proxies.json"
)

// --- App Configuration ---
type AppConfig struct {
	Proxies           []string `json:"proxies"`
	LastSelectedProxy string   `json:"last_selected_proxy"`
}

// --- Global State ---
var (
	tun2socksCmd         *exec.Cmd
	appConfig            AppConfig // Holds the entire application configuration
	proxies              []string  // Kept for convenience, mirrors appConfig.Proxies
	currentProxy         string
	mStart               *systray.MenuItem
	mStop                *systray.MenuItem
	mSelectProxy         *systray.MenuItem
	mDeleteProxy         *systray.MenuItem
	proxyMenuItems       map[string]*systray.MenuItem
	deleteProxyMenuItems map[string]*systray.MenuItem
	mu                   sync.RWMutex
	logFile              *os.File
)

//go:embed winres/icon.ico
var iconData []byte

func main() {
	// Set up logging to a file.
	var err error
	// Use O_APPEND to keep a running log across application restarts.
	logFile, err = os.OpenFile("TUNTray.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err == nil {
		// When compiled with -H=windowsgui, stdout is discarded, so writing to it
		// via MultiWriter can cause issues. We will log exclusively to the file.
		log.SetOutput(logFile)
	} else {
		// If we can't even open the log file, there's nowhere to log the error.
		// The application will still run, but without logging.
	}
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println("--- Application Starting ---")

	systray.Run(onReady, onExit)
}

func isElevated() bool {
	if runtime.GOOS != "windows" {
		return true // Not on Windows, assume it's fine
	}
	// The "net session" command will fail with "Access is denied" (error code 5)
	// if the user is not an administrator.
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}

func onReady() {
	if !isElevated() {
		zenity.Error("本程序需要管理员权限才能正常运行。\n请右键点击程序并选择“以管理员身份运行”。",
			zenity.Title("权限不足"),
			zenity.ErrorIcon)
		systray.Quit()
		return
	}

	systray.SetTitle("TUNTray")
	systray.SetTooltip("TUN 流量转发管理")
	systray.SetIcon(iconData)

	loadConfig()

	mStart = systray.AddMenuItem("启动", "启动 TUN")
	mStop = systray.AddMenuItem("停止", "停止 TUN")
	systray.AddSeparator()

	// --- Select Proxy Menu ---
	mSelectProxy = systray.AddMenuItem("选择代理", "选择一个代理服务器")
	proxyMenuItems = make(map[string]*systray.MenuItem)
	for _, p := range proxies {
		item := mSelectProxy.AddSubMenuItem(p, p)
		proxyMenuItems[p] = item
		go func(proxy string, menuItem *systray.MenuItem) {
			for {
				<-menuItem.ClickedCh
				if !mStart.Disabled() {
					setProxy(proxy)
				}
			}
		}(p, item)
	}

	// --- Manage Proxy Menu ---
	mManageProxies := systray.AddMenuItem("管理代理", "添加或删除代理")
	mAddNewProxy := mManageProxies.AddSubMenuItem("添加新代理...", "添加一个新的代理地址")
	mDeleteProxy = mManageProxies.AddSubMenuItem("删除代理", "删除一个现有的代理地址")
	deleteProxyMenuItems = make(map[string]*systray.MenuItem)
	if len(proxies) > 0 {
		for _, p := range proxies {
			item := mDeleteProxy.AddSubMenuItem(p, p)
			deleteProxyMenuItems[p] = item
			go func(proxy string, menuItem *systray.MenuItem) {
				for {
					<-menuItem.ClickedCh
					if !mStart.Disabled() {
						deleteProxy(proxy)
					}
				}
			}(p, item)
		}
	} else {
		mDeleteProxy.Disable()
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出程序")

	mStop.Disable()

	// Set initial proxy based on saved config
	initialProxy := ""
	if len(proxies) > 0 {
		// Check if the last selected proxy is still valid
		lastProxyIsValid := false
		for _, p := range proxies {
			if p == appConfig.LastSelectedProxy {
				lastProxyIsValid = true
				break
			}
		}

		if lastProxyIsValid {
			initialProxy = appConfig.LastSelectedProxy
		} else {
			initialProxy = proxies[0] // Fallback to the first one
		}
		setProxy(initialProxy)
	}

	// --- Main Event Loop ---
	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				handleStart()
			case <-mStop.ClickedCh:
				handleStop()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			case <-mAddNewProxy.ClickedCh:
				if !mStart.Disabled() {
					addNewProxy()
				}
			}
		}
	}()
}

func handleStart() {
	log.Println("启动 TUN...")
	if err := startTun(); err != nil {
		log.Printf("启动失败: %v\n", err)
	} else {
		log.Println("启动成功")
		mStart.Disable()
		mStop.Enable()
	}
}

func handleStop() {
	log.Println("停止 TUN...")
	if err := stopTun(); err != nil {
		log.Printf("停止失败: %v\n", err)
	} else {
		log.Println("停止成功")
		mStop.Disable()
		mStart.Enable()
	}
}

func onExit() {
	if tun2socksCmd != nil && tun2socksCmd.Process != nil {
		stopTun()
	}
	log.Println("--- Application Exiting ---")
	if logFile != nil {
		logFile.Close()
	}
	os.Exit(0)
}

// --- Proxy Management Logic ---

func setProxy(proxyAddr string) {
	mu.Lock()
	defer mu.Unlock()

	// If the proxy is already set, do nothing to prevent unnecessary file writes.
	if currentProxy == proxyAddr {
		if item, ok := proxyMenuItems[proxyAddr]; ok {
			item.Check()
		}
		return
	}

	currentProxy = proxyAddr
	appConfig.LastSelectedProxy = proxyAddr
	saveConfig() // Save config while holding the lock

	log.Printf("切换代理到: %s\n", currentProxy)

	// Update checkmarks
	for proxy, item := range proxyMenuItems {
		if proxy == proxyAddr {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func addNewProxy() {
	newProxy, err := zenity.Entry("请输入新的代理地址:",
		zenity.Title("添加新代理"),
		zenity.EntryText(""))
	if err != nil {
		if err == zenity.ErrCanceled {
			log.Println("用户取消了添加代理。")
		} else {
			log.Printf("无法打开输入框: %v\n", err)
		}
		return
	}

	newProxy = strings.TrimSpace(newProxy)
	if newProxy == "" {
		log.Println("输入的代理地址为空。")
		zenity.Warning("代理地址不能为空。", zenity.Title("输入无效"))
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Check for duplicates
	for _, p := range appConfig.Proxies {
		if p == newProxy {
			log.Printf("代理 '%s' 已存在。", newProxy)
			zenity.Warning("该代理地址已存在。", zenity.Title("添加失败"))
			return
		}
	}

	appConfig.Proxies = append(appConfig.Proxies, newProxy)
	proxies = appConfig.Proxies // Keep the convenience slice in sync
	saveConfig()

	// Dynamically add to "Select Proxy" menu
	itemSelect := mSelectProxy.AddSubMenuItem(newProxy, newProxy)
	proxyMenuItems[newProxy] = itemSelect
	go func(proxy string, menuItem *systray.MenuItem) {
		for {
			<-menuItem.ClickedCh
			if !mStart.Disabled() {
				setProxy(proxy)
			}
		}
	}(newProxy, itemSelect)

	// Dynamically add to "Delete Proxy" menu
	if mDeleteProxy.Disabled() {
		mDeleteProxy.Enable()
	}
	itemDelete := mDeleteProxy.AddSubMenuItem(newProxy, newProxy)
	deleteProxyMenuItems[newProxy] = itemDelete
	go func(proxy string, menuItem *systray.MenuItem) {
		for {
			<-menuItem.ClickedCh
			if !mStart.Disabled() {
				deleteProxy(proxy)
			}
		}
	}(newProxy, itemDelete)

	log.Printf("代理 '%s' 已添加, 菜单已更新。", newProxy)
	zenity.Info("代理已成功添加。", zenity.Title("操作成功"))
}

func deleteProxy(proxyAddr string) {
	mu.Lock()
	defer mu.Unlock()

	// --- Update data source ---
	newProxies := []string{}
	found := false
	for _, p := range appConfig.Proxies {
		if p != proxyAddr {
			newProxies = append(newProxies, p)
		} else {
			found = true
		}
	}

	// If the proxy to be deleted was not found, do nothing.
	if !found {
		return
	}

	appConfig.Proxies = newProxies
	proxies = appConfig.Proxies // Keep the convenience slice in sync

	// --- Update UI ---
	if item, ok := proxyMenuItems[proxyAddr]; ok {
		item.Hide()
		delete(proxyMenuItems, proxyAddr)
	}
	if item, ok := deleteProxyMenuItems[proxyAddr]; ok {
		item.Hide()
		delete(deleteProxyMenuItems, proxyAddr)
	}

	// If the deleted proxy was the current one, select a new one
	if currentProxy == proxyAddr {
		if len(appConfig.Proxies) > 0 {
			currentProxy = appConfig.Proxies[0]
			appConfig.LastSelectedProxy = currentProxy
			// Update checkmarks for the new proxy
			for p, item := range proxyMenuItems {
				if p == currentProxy {
					item.Check()
				} else {
					item.Uncheck()
				}
			}
		} else {
			currentProxy = ""
			appConfig.LastSelectedProxy = ""
			log.Println("所有代理均已删除。")
		}
	}

	saveConfig() // Save all changes

	if len(appConfig.Proxies) == 0 {
		mDeleteProxy.Disable()
	}

	log.Printf("代理 '%s' 已删除, 菜单已更新。", proxyAddr)
}

// --- File I/O for Config ---

func loadConfig() {
	mu.Lock()
	defer mu.Unlock()

	// Try to read the new config.json first
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, &appConfig); err == nil {
			log.Println("已成功加载 config.json。")
			proxies = appConfig.Proxies // Sync the convenience slice
			return
		}
		log.Printf("解析 config.json 失败: %v。将尝试迁移或创建默认配置。", err)
	}

	// If config.json doesn't exist or is corrupt, try to migrate from proxies.json
	data, err = os.ReadFile(oldProxiesFile)
	if err == nil {
		log.Println("找到旧的 proxies.json，正在迁移...")
		var oldProxies []string
		if json.Unmarshal(data, &oldProxies) == nil && len(oldProxies) > 0 {
			appConfig.Proxies = oldProxies
			appConfig.LastSelectedProxy = oldProxies[0] // Default to first
			proxies = appConfig.Proxies
			saveConfig()              // Save as new config.json
			os.Remove(oldProxiesFile) // Clean up old file
			log.Println("迁移成功，旧的 proxies.json 已删除。")
			return
		}
	}

	// If all else fails, create a default configuration
	log.Println("未找到有效配置, 创建默认配置...")
	appConfig.Proxies = []string{"socks5://127.0.0.1:7890"}
	if len(appConfig.Proxies) > 0 {
		appConfig.LastSelectedProxy = appConfig.Proxies[0]
	}
	proxies = appConfig.Proxies
	saveConfig()
}

func saveConfig() {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		log.Printf("无法编码 config.json: %v\n", err)
		return
	}
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		log.Printf("无法写入 config.json: %v\n", err)
	}
}

// --- Core TUN Logic ---

func startTun() error {
	if err := prepareWintunDll(); err != nil {
		return fmt.Errorf("准备 wintun.dll 失败: %w", err)
	}

	mu.RLock()
	proxy := currentProxy
	mu.RUnlock()
	if proxy == "" {
		return fmt.Errorf("未选择代理服务器")
	}
	tun2socksCmd = exec.Command("./tun2socks.exe", "-device", "wintun", "-proxy", proxy, "-loglevel", "info")
	if runtime.GOOS == "windows" {
		tun2socksCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	stdout, _ := tun2socksCmd.StdoutPipe()
	stderr, _ := tun2socksCmd.StderrPipe()
	go logPipe(stdout, "TUN2SOCKS_STDOUT")
	go logPipe(stderr, "TUN2SOCKS_STDERR")

	if err := tun2socksCmd.Start(); err != nil {
		return fmt.Errorf("启动 tun2socks.exe 失败: %w", err)
	}

	if err := waitForAdapter(); err != nil {
		tun2socksCmd.Process.Kill()
		return err
	}

	commands := []string{
		fmt.Sprintf("netsh interface ipv4 set address name=%s source=static addr=%s mask=%s", tunAlias, tunIP, tunMask),
		fmt.Sprintf("netsh interface ipv4 set dnsservers name=%s static address=%s register=none validate=no", tunAlias, tunDNS),
		fmt.Sprintf("netsh interface ipv4 add route 0.0.0.0/0 %s %s metric=1", tunAlias, tunIP),
	}
	for _, cmdStr := range commands {
		cmd := exec.Command("cmd", "/C", cmdStr)
		if output, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(cmdStr, "add route") {
				delCmdStr := fmt.Sprintf("netsh interface ipv4 delete route 0.0.0.0/0 %s", tunAlias)
				exec.Command("cmd", "/C", delCmdStr).Run()
				if output, err := exec.Command("cmd", "/C", cmdStr).CombinedOutput(); err != nil {
					return fmt.Errorf("执行命令 '%s' 失败: %s, %w", cmdStr, string(output), err)
				}
			} else {
				return fmt.Errorf("执行命令 '%s' 失败: %s, %w", cmdStr, string(output), err)
			}
		}
	}
	return nil
}

func stopTun() error {
	// 1. Clean up network settings with netsh
	netshCommands := []string{
		fmt.Sprintf("netsh interface ipv4 delete route 0.0.0.0/0 %s", tunAlias),
		fmt.Sprintf("netsh interface ipv4 set dnsservers name=%s source=dhcp", tunAlias),
		fmt.Sprintf("netsh interface ipv4 set address name=%s source=dhcp", tunAlias),
	}
	for _, cmdStr := range netshCommands {
		exec.Command("cmd", "/C", cmdStr).Run() // Ignore errors during cleanup
	}

	// 2. Clean up network profile from registry with PowerShell, just like the original script
	psCleanupCmd := `$profilesPath = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles'; if (Test-Path $profilesPath) { Get-ChildItem $profilesPath | ForEach-Object { try { $profile = Get-ItemProperty $_.PsPath; if ($profile.ProfileName -like 'wintun*') { Remove-Item $_.PsPath -Recurse -Force } } catch {} } }`
	exec.Command("powershell", "-Command", psCleanupCmd).Run() // Ignore errors during cleanup

	// 3. Stop the tun2socks.exe process
	if tun2socksCmd != nil && tun2socksCmd.Process != nil {
		if err := tun2socksCmd.Process.Kill(); err != nil {
			return fmt.Errorf("停止 tun2socks.exe 进程失败: %w", err)
		}
		tun2socksCmd = nil
	}
	return nil
}

func prepareWintunDll() error {
	dst := "./wintun.dll"
	// If wintun.dll already exists in the target location, do nothing.
	// This handles the distributed case where the DLL is already alongside the exe.
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	// If it doesn't exist (development environment), copy it from the arch-specific folder.
	var arch string
	if runtime.GOARCH == "amd64" {
		arch = "x64"
	} else {
		arch = "x86"
	}
	src := fmt.Sprintf("./wintun/%s/wintun.dll", arch)

	sourceFile, err := os.ReadFile(src)
	if err != nil {
		// This error is now expected in a distributed environment, but indicates a problem
		// in a dev environment if the wintun/ folder is missing.
		return fmt.Errorf("wintun.dll 不存在于当前目录，也无法从 %s 复制: %w", src, err)
	}
	if err := os.WriteFile(dst, sourceFile, 0666); err != nil {
		return fmt.Errorf("复制 wintun.dll 失败 (%s): %w", dst, err)
	}
	log.Println("已从开发目录复制 wintun.dll。")
	return nil
}

func waitForAdapter() error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return fmt.Errorf("获取网络接口失败: %w", err)
		}
		for _, i := range interfaces {
			if i.Name == tunAlias {
				log.Printf("网络适配器 '%s' 已找到\n", tunAlias)
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("等待网络适配器 '%s' 超时", tunAlias)
}

func logPipe(pipe io.ReadCloser, prefix string) {
	if pipe == nil {
		return
	}
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		log.Printf("[%s] %s\n", prefix, scanner.Text())
	}
}
