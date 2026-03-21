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

### Hammerspoon（可选，备用方案）

如果不想运行 GUI，也可以通过 Hammerspoon 配合 CLI 使用快捷键：

```bash
./restart.sh
```

此脚本会检查/安装 Hammerspoon、复制配置并重启。

## 项目结构

```
word-collector/
├── cmd/
│   ├── cli/main.go              # CLI 主程序
│   └── gui/
│       ├── main.go              # GUI 界面（Fyne）
│       └── hotkey_darwin.go     # macOS 全局快捷键（CGO）
├── build-dmg.sh                 # DMG 打包脚本
├── restart.sh                   # Hammerspoon 重启脚本（可选）
├── hammerspoon/                 # Hammerspoon 配置（可选）
│   ├── word_collector.lua
│   └── install.sh
└── dist/
    ├── Word Collector.app       # 打包的应用
    └── WordCollector-*.dmg      # DMG 安装包
```

## Anki 配置

CLI 版本默认使用：
- **牌组**：系统默认
- **模板**：问答题
- **标签**：word-collector
- **导出文件**：`~/word-collector/anki_import.txt`

GUI 版本默认使用：
- **牌组**：Default
- **模板**：Basic
- **标签**：word-collector
- **导出文件**：`~/word-collector/anki_import.txt`

如需修改，请编辑 `cmd/cli/main.go` 或 `cmd/gui/main.go` 中的常量。

### AnkiConnect 插件（推荐）

要实现一键自动导入 Anki，安装 AnkiConnect 插件：

1. 打开 Anki → 工具 → 附加组件 → 获取插件
2. 输入代码：`2055492159`
3. 重启 Anki

## 依赖

- Go 1.21+
- [Fyne v2.4.3](https://fyne.io/)（GUI）
- [Anki](https://apps.ankiweb.net/) + AnkiConnect 插件（可选）
- [Hammerspoon](https://www.hammerspoon.org/)（可选，仅 CLI 快捷键方案需要）

## 故障排除

### 快捷键不工作

1. 确认 GUI 正在运行（关闭窗口后会最小化到系统托盘）
2. 检查辅助功能权限：系统设置 → 隐私与安全性 → 辅助功能，确保 Word Collector 已勾选
3. 如果从终端启动，需要勾选终端应用（如 Terminal.app 或 iTerm）的辅助功能权限
4. 重启应用后重试

### 无法添加到 Anki

1. 确认 Anki 已安装 AnkiConnect 插件
2. 确认 Anki 正在运行
3. 检查 AnkiConnect 地址：`http://localhost:8765`

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

## 数据格式

生成的 `anki_import.txt` 使用 Tab 分隔：

```
正面内容(英文+音标)\t背面内容(中文释义)\t标签
```

## License

MIT
