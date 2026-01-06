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
	Language          Language `json:"language"`
}

// initializeLanguage sets up the language based on config or system default
func initializeLanguage(configLoaded bool) {
	mu.Lock()
	defer mu.Unlock()

	log.Printf("Initializing language. Config language: %v, configLoaded: %v\n", appConfig.Language, configLoaded)

	// If language is not set in config (Unset), default to English
	if appConfig.Language == Unset {
		log.Println("Language not set in config, defaulting to English...")
		appConfig.Language = English  // Default to English
		SetLanguage(appConfig.Language)
		saveConfig()
		log.Printf("Default language set to: %v\n", appConfig.Language)
	} else {
		SetLanguage(appConfig.Language)
		log.Printf("Language loaded from config: %v\n", appConfig.Language)
	}

	// For existing configs that don't have language field, we can't detect it
	// So we assume language=0 means Chinese was intentionally chosen
	// If you want to force English for all existing users, uncomment the following:
	// if configLoaded && appConfig.Language == Chinese {
	//     log.Println("Existing config detected, defaulting to English for all users...")
	//     appConfig.Language = English
	//     SetLanguage(appConfig.Language)
	//     saveConfig()
	// }
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
	mManageProxies       *systray.MenuItem
	mAddNewProxy         *systray.MenuItem
	mQuit                *systray.MenuItem
	mLanguage            *systray.MenuItem
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
		zenity.Error(GetText("permission_error_msg"),
			zenity.Title(GetText("permission_error_title")),
			zenity.ErrorIcon)
		systray.Quit()
		return
	}

	systray.SetIcon(iconData)

	configLoaded, _ := loadConfig()

	// Initialize language settings
	initializeLanguage(configLoaded)

	systray.SetTitle(GetText("app_title"))
	systray.SetTooltip(GetText("app_tooltip"))

	mStart = systray.AddMenuItem(GetText("start"), GetText("start_tooltip"))
	mStop = systray.AddMenuItem(GetText("stop"), GetText("stop_tooltip"))
	systray.AddSeparator()

	// --- Select Proxy Menu ---
	mSelectProxy = systray.AddMenuItem(GetText("select_proxy"), GetText("select_tooltip"))
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
	mManageProxies = systray.AddMenuItem(GetText("manage_proxies"), GetText("manage_tooltip"))
	mAddNewProxy = mManageProxies.AddSubMenuItem(GetText("add_new_proxy"), GetText("add_tooltip"))
	mDeleteProxy = mManageProxies.AddSubMenuItem(GetText("delete_proxy"), GetText("delete_tooltip"))
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

	// --- Language Menu ---
	_, languageSubMenus := createLanguageMenu()
	go handleLanguageSelection(languageSubMenus)

	systray.AddSeparator()
	mQuit = systray.AddMenuItem(GetText("quit"), "Quit program")

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
	log.Println(GetText("log_starting"))
	if err := startTun(); err != nil {
		log.Printf(GetText("start_fail")+": %v\n", err)
	} else {
		log.Println(GetText("start_success"))
		mStart.Disable()
		mStop.Enable()
	}
}

func handleStop() {
	log.Println(GetText("log_stopping"))
	if err := stopTun(); err != nil {
		log.Printf(GetText("stop_fail")+": %v\n", err)
	} else {
		log.Println(GetText("stop_success"))
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

	log.Printf(GetText("log_proxy_switch")+"\n", currentProxy)

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
	newProxy, err := zenity.Entry(GetText("add_proxy_prompt"),
		zenity.Title(GetText("add_proxy_title")),
		zenity.EntryText(""))
	if err != nil {
		if err == zenity.ErrCanceled {
			log.Println(GetText("user_cancelled"))
		} else {
			log.Printf(GetText("cannot_open_input")+"\n", err)
		}
		return
	}

	newProxy = strings.TrimSpace(newProxy)
	if newProxy == "" {
		log.Println(GetText("proxy_empty_error"))
		zenity.Warning(GetText("proxy_empty_error"), zenity.Title(GetText("input_invalid")))
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Check for duplicates
	for _, p := range appConfig.Proxies {
		if p == newProxy {
			log.Printf(GetText("proxy_exists_error")+" ", newProxy)
			zenity.Warning(GetText("proxy_exists_error"), zenity.Title(GetText("add_proxy_failed")))
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

	log.Printf(GetText("log_proxy_added")+"", newProxy)
	zenity.Info(GetText("add_proxy_success"), zenity.Title(GetText("operation_success")))
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
			log.Println(GetText("log_all_proxies_deleted"))
		}
	}

	saveConfig() // Save all changes

	if len(appConfig.Proxies) == 0 {
		mDeleteProxy.Disable()
	}

	log.Printf(GetText("log_proxy_deleted")+"", proxyAddr)
}

// --- File I/O for Config ---

func loadConfig() (bool, error) {
	mu.Lock()
	defer mu.Unlock()

	// Try to read the new config.json first
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, &appConfig); err == nil {
			log.Println(GetText("config_load_success"))
			proxies = appConfig.Proxies // Sync the convenience slice
			// Config loaded successfully, check if language field was present
			return true, nil
		}
		log.Printf(GetText("config_parse_fail")+"\n", err)
		return false, err
	}

	// If config.json doesn't exist or is corrupt, try to migrate from proxies.json
	data, err = os.ReadFile(oldProxiesFile)
	if err == nil {
		log.Println(GetText("migration_start"))
		var oldProxies []string
		if json.Unmarshal(data, &oldProxies) == nil && len(oldProxies) > 0 {
			appConfig.Proxies = oldProxies
			appConfig.LastSelectedProxy = oldProxies[0] // Default to first
			appConfig.Language = Unset                 // New config, language not set
			proxies = appConfig.Proxies
			saveConfig()              // Save as new config.json
			os.Remove(oldProxiesFile) // Clean up old file
			log.Println(GetText("migration_success"))
			return false, nil
		}
	}

	// If all else fails, create a default configuration
	log.Println(GetText("no_valid_config"))
	appConfig.Proxies = []string{"socks5://127.0.0.1:7890"}
	if len(appConfig.Proxies) > 0 {
		appConfig.LastSelectedProxy = appConfig.Proxies[0]
	}
	appConfig.Language = Unset // New config, language not set
	proxies = appConfig.Proxies
	saveConfig()
	return false, nil
}

func saveConfig() {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		log.Printf(GetText("config_encode_fail")+"\n", err)
		return
	}
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		log.Printf(GetText("config_write_fail")+"\n", err)
	}
}

// --- Core TUN Logic ---

func startTun() error {
	if err := prepareWintunDll(); err != nil {
		return fmt.Errorf(GetTextWithFormat("prepare_wintun_fail"), err)
	}

	mu.RLock()
	proxy := currentProxy
	mu.RUnlock()
	if proxy == "" {
		return fmt.Errorf(GetText("no_proxy_selected"))
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
		return fmt.Errorf(GetTextWithFormat("start_tun2socks_fail"), err)
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
					return fmt.Errorf(GetTextWithFormat("command_exec_fail"), cmdStr, string(output), err)
				}
			} else {
				return fmt.Errorf(GetTextWithFormat("command_exec_fail"), cmdStr, string(output), err)
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
			return fmt.Errorf(GetTextWithFormat("stop_tun2socks_fail"), err)
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
		return fmt.Errorf(GetTextWithFormat("wintun_not_found"), src, err)
	}
	if err := os.WriteFile(dst, sourceFile, 0666); err != nil {
		return fmt.Errorf(GetTextWithFormat("copy_wintun_fail"), dst, err)
	}
	log.Println(GetText("copy_wintun_success"))
	return nil
}

func waitForAdapter() error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return fmt.Errorf(GetTextWithFormat("get_interfaces_fail"), err)
		}
		for _, i := range interfaces {
			if i.Name == tunAlias {
				log.Printf(GetText("adapter_found")+"\n", tunAlias)
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf(GetTextWithFormat("wait_adapter_timeout"), tunAlias)
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

// createLanguageMenu creates the language selection menu and returns menu items
func createLanguageMenu() (*systray.MenuItem, map[Language]*systray.MenuItem) {
	// Remove old language menu if it exists (for refresh)
	// Note: systray doesn't support removing items, so we just create new ones
	// The old menu will be garbage collected when the program restarts

	// Create the main language menu
	mLanguage = systray.AddMenuItem(GetText("language"), "Switch language")
	mLanguageChinese := mLanguage.AddSubMenuItem(GetText("chinese"), "Switch to Chinese")
	mLanguageEnglish := mLanguage.AddSubMenuItem(GetText("english"), "Switch to English")

	subMenus := map[Language]*systray.MenuItem{
		Chinese:  mLanguageChinese,
		English: mLanguageEnglish,
	}

	// Update checkmarks based on current language
	updateLanguageCheckmarks(subMenus)

	return mLanguage, subMenus
}

// handleLanguageSelection handles language selection events
func handleLanguageSelection(subMenus map[Language]*systray.MenuItem) {
	for {
		select {
		case <-subMenus[Chinese].ClickedCh:
			log.Println("Language switch requested: Chinese")
			SetLanguage(Chinese)
			appConfig.Language = Chinese
			saveConfig()
			log.Printf("Language switched to Chinese, config updated. Current language: %v\n", GetCurrentLanguage())
			updateLanguageCheckmarks(subMenus)
			refreshUITexts()
			zenity.Info(GetText("operation_success"), zenity.Title(GetText("language")))
		case <-subMenus[English].ClickedCh:
			log.Println("Language switch requested: English")
			SetLanguage(English)
			appConfig.Language = English
			saveConfig()
			log.Printf("Language switched to English, config updated. Current language: %v\n", GetCurrentLanguage())
			updateLanguageCheckmarks(subMenus)
			refreshUITexts()
			zenity.Info(GetText("operation_success"), zenity.Title(GetText("language")))
		}
	}
}

// updateLanguageCheckmarks updates the checkmarks on language menu items
func updateLanguageCheckmarks(subMenus map[Language]*systray.MenuItem) {
	currentLang := GetCurrentLanguage()
	for lang, menu := range subMenus {
		if lang == currentLang {
			menu.Check()
		} else {
			menu.Uncheck()
		}
	}
}

// refreshUITexts refreshes UI texts that can be updated dynamically
func refreshUITexts() {
	log.Println("Refreshing UI texts for new language...")

	// Update main menu items
	if mStart != nil {
		mStart.SetTitle(GetText("start"))
		mStart.SetTooltip(GetText("start_tooltip"))
	}
	if mStop != nil {
		mStop.SetTitle(GetText("stop"))
		mStop.SetTooltip(GetText("stop_tooltip"))
	}
	if mSelectProxy != nil {
		mSelectProxy.SetTitle(GetText("select_proxy"))
		mSelectProxy.SetTooltip(GetText("select_tooltip"))
	}
	if mDeleteProxy != nil {
		mDeleteProxy.SetTitle(GetText("delete_proxy"))
		mDeleteProxy.SetTooltip(GetText("delete_tooltip"))
	}
	if mManageProxies != nil {
		mManageProxies.SetTitle(GetText("manage_proxies"))
		mManageProxies.SetTooltip(GetText("manage_tooltip"))
	}
	if mAddNewProxy != nil {
		mAddNewProxy.SetTitle(GetText("add_new_proxy"))
		mAddNewProxy.SetTooltip(GetText("add_tooltip"))
	}
	if mQuit != nil {
		mQuit.SetTitle(GetText("quit"))
	}
	if mLanguage != nil {
		mLanguage.SetTitle(GetText("language"))
		mLanguage.SetTooltip("Switch language")
	}

	// Update the title and tooltip of the main app
	systray.SetTitle(GetText("app_title"))
	systray.SetTooltip(GetText("app_tooltip"))

	log.Println("UI texts refreshed successfully")
}
