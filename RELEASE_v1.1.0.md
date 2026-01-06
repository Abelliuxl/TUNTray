# TUNTray v1.1.0 Release Notes

## 新功能与改进 (New Features & Improvements)

### 1. 双语切换功能 (Bilingual Switching)
- 实现了完整的中英文双语界面切换
- 支持动态更新UI文本，无需重启应用程序
- 添加了语言选择菜单，可实时切换中文/英文

### 2. 语言系统增强 (Language System Enhancement)
- 新增 `Unset` 语言常量（值255），用于标识未设置的语言状态
- 改进了语言初始化逻辑，默认使用英语而非系统语言
- 添加了动态UI文本刷新机制，切换语言时自动更新所有菜单项

### 3. 配置处理改进 (Configuration Handling Improvements)
- 改进了 `loadConfig()` 函数，返回配置加载状态
- 支持从旧版 `proxies.json` 迁移到新版 `config.json`
- 语言设置现在持久化保存，重启后保持选择

### 4. 构建脚本 (Build Script)
- 添加了 Windows GUI 构建脚本 (`build.bat`)
- 简化了构建流程，一键完成资源生成、编译和文件复制

### 5. 文档更新 (Documentation Updates)
- 更新了 `.gitignore` 文件，忽略构建产物和工具文件
- 完善了 README 中的构建说明

## 技术细节 (Technical Details)

### 主要变更文件：
- `language.go`：添加了 `Unset` 常量和新的翻译键
- `main.go`：重构了语言初始化、添加了动态UI刷新、改进了配置处理
- `build.bat`：新增的构建脚本
- `.gitignore`：更新了忽略规则

### 提交记录：
- `4c22610` Improve language system: add Unset constant, dynamic UI refresh, and better config handling
- `15729ce` docs: 添加Windows GUI构建脚本
- `8ccb93f` feat: 实现中英文双语切换功能
- `eb53f1f` docs: 更新.gitignore忽略构建产物和工具文件

## 使用说明 (Usage Instructions)

### 语言切换：
1. 点击系统托盘图标
2. 选择"语言" (Language) 菜单
3. 选择"中文"或"English"
4. 界面文本将立即更新，无需重启

### 构建：
- 运行 `build.bat` 或按照 README 中的 PowerShell 命令进行构建

## 下载 (Downloads)

预编译的二进制文件可在 [GitHub Releases](https://github.com/Abelliuxl/TUNTray/releases) 页面下载。

## 致谢 (Acknowledgments)

特别感谢 [xjasonlyu/tun2socks](https://github.com/xjasonlyu/tun2socks) 项目提供核心功能。

---

**发布日期：** 2026-01-06  
**版本：** v1.1.0  
**上一个版本：** v1.0.0