package main

import (
	"fmt"
	"syscall"
)

// Windows API constants for language detection
var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	getSystemDefaultUILanguage = kernel32.NewProc("GetSystemDefaultUILanguage")
)

// getWindowsSystemLanguage returns the Windows system language ID
func getWindowsSystemLanguage() uint16 {
	// GetSystemDefaultUILanguage returns the language ID of the system UI language
	langID, _, _ := getSystemDefaultUILanguage.Call()
	return uint16(langID)
}

// Language represents the language preference
type Language int

const (
	Chinese Language = iota
	English
	Unset Language = 255 // Special value to indicate language is not set
)

// String returns the string representation of the language
func (l Language) String() string {
	switch l {
	case Chinese:
		return "Chinese"
	case English:
		return "English"
	default:
		return "Unknown"
	}
}

// LanguageMap contains all translated strings for all supported languages
type LanguageMap map[Language]map[string]string

// Translations holds all language translations
var Translations = LanguageMap{
	Chinese: {
		// Menu items
		"start":            "启动",
		"stop":             "停止",
		"select_proxy":     "选择代理",
		"manage_proxies":   "管理代理",
		"add_new_proxy":    "添加新代理...",
		"delete_proxy":     "删除代理",
		"language":         "语言",
		"chinese":          "中文",
		"english":          "English",
		"quit":             "退出",

		// Tooltips and titles
		"app_title":        "TUNTray",
		"app_tooltip":     "TUN 流量转发管理",
		"start_tooltip":   "启动 TUN",
		"stop_tooltip":    "停止 TUN",
		"select_tooltip":  "选择一个代理服务器",
		"add_tooltip":     "添加一个新的代理地址",
		"delete_tooltip":  "删除一个现有的代理地址",
		"manage_tooltip":  "添加或删除代理",

		// Error messages
		"permission_error_title":  "权限不足",
		"permission_error_msg":   "本程序需要管理员权限才能正常运行。\n请右键点击程序并选择\"以管理员身份运行\"。",

		// Operation messages
		"start_success":   "启动成功",
		"start_fail":      "启动失败",
		"stop_success":    "停止成功",
		"stop_fail":       "停止失败",

		// Log messages
		"log_starting":    "启动 TUN...",
		"log_stopping":    "停止 TUN...",
		"log_proxy_switch": "切换代理到: %s",
		"log_proxy_added":  "代理 '%s' 已添加, 菜单已更新。",
		"log_proxy_deleted": "代理 '%s' 已删除, 菜单已更新。",
		"log_all_proxies_deleted": "所有代理均已删除。",

		// Dialog messages
		"add_proxy_title":      "添加新代理",
		"add_proxy_prompt":     "请输入新的代理地址:",
		"proxy_empty_error":    "代理地址不能为空。",
		"proxy_exists_error":    "该代理地址已存在。",
		"add_proxy_failed":      "添加失败",
		"input_invalid":         "输入无效",
		"add_proxy_success":     "代理已成功添加。",
		"operation_success":    "操作成功",
		"user_cancelled":        "用户取消了添加代理。",
		"cannot_open_input":     "无法打开输入框: %v",

		// Config messages
		"config_load_success":  "已成功加载 config.json。",
		"config_parse_fail":    "解析 config.json 失败: %v。将尝试迁移或创建默认配置。",
		"migration_start":      "找到旧的 proxies.json，正在迁移...",
		"migration_success":     "迁移成功，旧的 proxies.json 已删除。",
		"no_valid_config":      "未找到有效配置, 创建默认配置...",
		"config_encode_fail":    "无法编码 config.json: %v",
		"config_write_fail":    "无法写入 config.json: %v",

		// Core logic messages
		"prepare_wintun_fail":  "准备 wintun.dll 失败: %w",
		"no_proxy_selected":    "未选择代理服务器",
		"start_tun2socks_fail": "启动 tun2socks.exe 失败: %w",
		"stop_tun2socks_fail":  "停止 tun2socks.exe 失败: %w",
		"wait_adapter_timeout": "等待网络适配器 '%s' 超时",
		"adapter_found":        "网络适配器 '%s' 已找到",
		"get_interfaces_fail":  "获取网络接口失败: %w",
		"copy_wintun_success":  "已从开发目录复制 wintun.dll。",
		"copy_wintun_fail":     "复制 wintun.dll 失败 (%s): %w",
		"wintun_not_found":     "wintun.dll 不存在于当前目录，也无法从 %s 复制: %w",
		"command_exec_fail":    "执行命令 '%s' 失败: %s, %w",
	},
	English: {
		// Menu items
		"start":            "Start",
		"stop":             "Stop",
		"select_proxy":     "Select Proxy",
		"manage_proxies":   "Manage Proxies",
		"add_new_proxy":    "Add New Proxy...",
		"delete_proxy":     "Delete Proxy",
		"language":         "Language",
		"chinese":          "中文",
		"english":          "English",
		"quit":             "Quit",

		// Tooltips and titles
		"app_title":        "TUNTray",
		"app_tooltip":     "TUN Traffic Forwarding Manager",
		"start_tooltip":   "Start TUN",
		"stop_tooltip":    "Stop TUN",
		"select_tooltip":  "Select a proxy server",
		"add_tooltip":     "Add a new proxy address",
		"delete_tooltip":  "Delete an existing proxy address",
		"manage_tooltip":  "Add or delete proxies",

		// Error messages
		"permission_error_title":  "Insufficient Privileges",
		"permission_error_msg":   "This program requires administrator privileges to run.\nPlease right-click and select \"Run as administrator\".",

		// Operation messages
		"start_success":   "Started successfully",
		"start_fail":      "Failed to start",
		"stop_success":    "Stopped successfully",
		"stop_fail":       "Failed to stop",

		// Log messages
		"log_starting":    "Starting TUN...",
		"log_stopping":    "Stopping TUN...",
		"log_proxy_switch": "Switched proxy to: %s",
		"log_proxy_added":  "Proxy '%s' added, menu updated.",
		"log_proxy_deleted": "Proxy '%s' deleted, menu updated.",
		"log_all_proxies_deleted": "All proxies have been deleted.",

		// Dialog messages
		"add_proxy_title":      "Add New Proxy",
		"add_proxy_prompt":     "Enter new proxy address:",
		"proxy_empty_error":    "Proxy address cannot be empty.",
		"proxy_exists_error":    "This proxy address already exists.",
		"add_proxy_failed":      "Add Failed",
		"input_invalid":         "Invalid Input",
		"add_proxy_success":     "Proxy added successfully.",
		"operation_success":    "Success",
		"user_cancelled":        "User cancelled adding proxy.",
		"cannot_open_input":     "Cannot open input dialog: %v",

		// Config messages
		"config_load_success":  "Successfully loaded config.json.",
		"config_parse_fail":    "Failed to parse config.json: %v. Will try to migrate or create default configuration.",
		"migration_start":      "Found old proxies.json, migrating...",
		"migration_success":     "Migration successful, old proxies.json deleted.",
		"no_valid_config":      "No valid configuration found, creating default...",
		"config_encode_fail":    "Cannot encode config.json: %v",
		"config_write_fail":    "Cannot write config.json: %v",

		// Core logic messages
		"prepare_wintun_fail":  "Failed to prepare wintun.dll: %w",
		"no_proxy_selected":    "No proxy server selected",
		"start_tun2socks_fail": "Failed to start tun2socks.exe: %w",
		"stop_tun2socks_fail":  "Failed to stop tun2socks.exe: %w",
		"wait_adapter_timeout": "Waiting for network adapter '%s' timed out",
		"adapter_found":        "Network adapter '%s' found",
		"get_interfaces_fail":  "Failed to get network interfaces: %w",
		"copy_wintun_success":  "Copied wintun.dll from development directory.",
		"copy_wintun_fail":     "Failed to copy wintun.dll (%s): %w",
		"wintun_not_found":     "wintun.dll does not exist in current directory and cannot be copied from %s: %w",
		"command_exec_fail":    "Failed to execute command '%s': %s, %w",
	},
}

// currentLanguage stores the current language setting
var currentLanguage Language = Chinese

// GetText returns the translated text for the given key in the current language
func GetText(key string) string {
	if langMap, exists := Translations[currentLanguage]; exists {
		if text, found := langMap[key]; found {
			return text
		}
	}
	// Fallback to Chinese if translation not found
	if langMap, exists := Translations[Chinese]; exists {
		if text, found := langMap[key]; found {
			return text
		}
	}
	return key
}

// GetTextWithFormat returns the translated text formatted with the given arguments
func GetTextWithFormat(key string, args ...interface{}) string {
	text := GetText(key)
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// GetSystemLanguage detects the system language and returns the appropriate Language
func GetSystemLanguage() Language {
	// Get Windows system language ID
	langID := getWindowsSystemLanguage()

	// Language ID mapping
	// 0x0409 = 1033 = English (US)
	// 0x0804 = 2052 = Chinese (Simplified, PRC)
	// 0x0404 = 1028 = Chinese (Traditional, Taiwan)
	// 0x0C04 = 3076 = Chinese (Traditional, Hong Kong SAR)
	switch langID {
	case 0x0409, 0x0809, 0x0C09, 0x1009, 0x1409, 0x1809, 0x1C09, 0x2009, 0x2409, 0x2809, 0x2C09, 0x3009, 0x3409:
		// English variants
		return English
	case 0x0404, 0x0804, 0x0C04, 0x1004, 0x1404, 0x1804, 0x1C04, 0x2004, 0x2404, 0x2804, 0x2C04, 0x3004, 0x3404, 0x3804:
		// Chinese variants
		return Chinese
	default:
		// Default to Chinese for non-enumerated languages
		return Chinese
	}
}

// SetLanguage sets the current language
func SetLanguage(lang Language) {
	currentLanguage = lang
}

// GetCurrentLanguage returns the current language
func GetCurrentLanguage() Language {
	return currentLanguage
}

// GetLanguageName returns the name of the language in the current language
func GetLanguageName(lang Language) string {
	switch lang {
	case Chinese:
		return "中文"
	case English:
		return "English"
	default:
		return "Unknown"
	}
}
