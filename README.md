# Word Collector

macOS 单词收集工具，帮助你在阅读时快速收集生词并添加到 Anki。

## 功能

- **快捷键取词**：选中单词后按快捷键自动添加到 Anki
- **GUI 界面**：使用 Fyne 构建的图形界面
- **多翻译源**：支持有道词典等翻译 API
- **Anki 集成**：自动添加到 Anki 或导出为文件
- **暂停/启用**：随时暂停或启用取词功能

## 安装

### 方式一：DMG 安装（推荐）

```bash
./build-dmg.sh
```

然后打开 `dist/WordCollector-1.0.0.dmg`，将应用拖到 Applications 文件夹。

### 方式二：手动安装

```bash
# 编译 CLI 版本
go build -o word-collector ./cmd/cli/

# 编译 GUI 版本（需要 CGO）
go build -o word-collector-gui ./cmd/gui/

# 安装 CLI 到 ~/word-collector/
mkdir -p ~/word-collector
cp word-collector ~/word-collector/
```

## 使用方法

### GUI 界面

```bash
open "dist/Word Collector.app"
# 或
./word-collector-gui
```

GUI 功能：
| 功能 | 说明 |
|------|------|
| **启用/暂停** | 切换取词功能状态 |
| **单词输入框** | 输入单词后按回车添加 |
| **粘贴按钮** | 从剪贴板粘贴单词 |
| **打开 Anki** | 启动 Anki 应用 |
| **打开文件夹** | 打开单词收集目录 |
| **系统托盘** | 关闭窗口后最小化到托盘 |

### 全局快捷键（GUI 内置）

GUI 运行时自动注册全局快捷键，无需安装任何外部工具。

首次使用需授予辅助功能权限：
- 系统设置 → 隐私与安全性 → 辅助功能
- 勾选 Word Collector（或终端应用，如果从终端启动）

| 快捷键 | 功能 |
|--------|------|
| `⌃⌥⌘W` | 选中单词后取词添加到 Anki |
| `⌃⌥⌘S` | 暂停/启用取词功能 |

### CLI 命令行

```bash
# 直接查询单词
~/word-collector/word-collector hello

# 从剪贴板/选中文本取词（无参数时自动获取）
~/word-collector/word-collector
```

## 项目结构

```
word-collector/
├── cmd/
│   ├── cli/main.go              # CLI 主程序
│   └── gui/
│       ├── main.go              # GUI 界面（Fyne）
│       ├── hotkey_darwin.go     # macOS 全局快捷键（CGO）
│       ├── ankiconnect.go      # AnkiConnect 插件自动安装
│       ├── config.go            # Anki 配置管理
│       └── settings.go          # 设置对话框
├── build-dmg.sh                 # DMG 打包脚本
└── dist/
    ├── Word Collector.app       # 打包的应用
    └── WordCollector-*.dmg      # DMG 安装包
```

## Anki 配置

GUI 版本：
- **首次运行**时会弹出设置对话框，从 Anki 获取牌组/模板列表让你选择
- 配置保存在 `~/.wordcollector_config.json`
- 随时点击 **⚙ Settings** 按钮修改牌组
- **标签**：word-collector
- **去重**：自动检查 Anki 中是否已存在相同单词

CLI 版本默认使用：
- **牌组**：系统默认
- **模板**：问答题
- **标签**：word-collector

如需修改 CLI 配置，请编辑 `cmd/cli/main.go` 中的常量。

### AnkiConnect 插件

GUI 首次启动时会自动安装 AnkiConnect 插件，无需手动操作。安装后需重启 Anki 一次以激活插件。

## 依赖

- Go 1.21+
- [Fyne v2.4.3](https://fyne.io/)（GUI）
- [Anki](https://apps.ankiweb.net/)（AnkiConnect 插件自动安装）

## 故障排除

### 快捷键不工作

1. 确认 GUI 正在运行（关闭窗口后会最小化到系统托盘）
2. 检查辅助功能权限：系统设置 → 隐私与安全性 → 辅助功能，确保 Word Collector 已勾选
3. 如果从终端启动，需要勾选终端应用（如 Terminal.app 或 iTerm）的辅助功能权限
4. 重启应用后重试

### 无法添加到 Anki

1. 确认 Anki 正在运行（按快捷键时会自动启动 Anki）
2. 确认已通过 ⚙ Settings 按钮配置了正确的牌组和模板
3. 检查 AnkiConnect 是否激活：`curl http://localhost:8765` 应返回 `AnkiConnect`
4. 如 AnkiConnect 未激活，重启 Anki 一次

### GUI 无法启动

确保已安装 Fyne 的系统依赖：
```bash
brew install pkg-config
```

### 编译错误

GUI 版本需要 CGO，不能交叉编译：
```bash
# 正确方式（当前架构）
CGO_ENABLED=1 go build ./cmd/gui/

# 错误方式（交叉编译不支持）
GOARCH=amd64 go build ./cmd/gui/  # 会失败
```

## 开发

```bash
# 开发模式运行 CLI
go run ./cmd/cli/ "hello"

# 开发模式运行 GUI
go run ./cmd/gui/

# 构建 CLI（通用二进制）
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o word-collector-arm64 ./cmd/cli/
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o word-collector-amd64 ./cmd/cli/
lipo -create word-collector-arm64 word-collector-amd64 -output word-collector

# 构建 GUI（仅当前架构）
CGO_ENABLED=1 go build -ldflags="-s -w" -o word-collector-gui ./cmd/gui/

# 格式化代码
go fmt ./...
```

## License

MIT
