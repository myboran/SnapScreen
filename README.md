# SnapScreen

一个基于 WebRTC 的实时屏幕共享应用，支持在局域网内快速分享和观看屏幕内容。

## ✨ 特性

- 🖥️ **双模式支持**
  - **Publisher（分享屏幕）**：将本地屏幕内容实时推流
  - **Viewer（观看屏幕）**：接收并观看远程屏幕内容

- 🔍 **局域网自动发现**
  - Publisher 自动通过 UDP 广播服务信息
  - Viewer 自动发现局域网内的可用 Publisher
  - 无需手动配置 IP 地址

- ⚙️ **灵活的配置选项**
  - 支持多屏幕选择
  - 可配置帧率（默认 30fps，建议 20-30fps）
  - 可自定义输出分辨率
  - 支持区域捕获（指定屏幕区域）

- 🚀 **内嵌信令服务器**
  - Publisher 模式自动启动内嵌信令服务器
  - 无需额外部署信令服务
  - 支持手动指定外部信令服务器

- 📡 **WebRTC 实时传输**
  - 使用 WebRTC DataChannel 传输屏幕帧
  - JPEG 编码，平衡画质与性能
  - 低延迟实时传输

## 📋 系统要求

- **操作系统**：Windows / Linux / macOS
- **Go 版本**：1.25.1 或更高
- **依赖项**：见 `go.mod`

## 🚀 快速开始

### 安装依赖

```bash
go mod download
```

### 编译运行

```bash
go run main.go
```

或编译为可执行文件：

```bash
go build -o snap-screen main.go
./snap-screen  # Linux/macOS
# 或 snap-screen.exe  # Windows
```

## 📖 使用指南

### Publisher 模式（分享屏幕）

1. 启动应用，选择 **"分享屏幕"**
2. 配置参数：
   - **Stream ID**：自动生成或手动输入（用于标识流）
   - **信令服务器**：默认使用内嵌服务器（自动启动）
   - **屏幕选择**：选择要分享的屏幕
   - **帧率**：设置推流帧率（默认 30fps）
   - **输出分辨率**：可选，留空则使用原始分辨率
   - **区域捕获**：可选，启用后可指定屏幕区域（x, y, width, height）
3. 点击 **"开始分享"**
4. 等待 Viewer 连接并开始观看

### Viewer 模式（观看屏幕）

1. 启动应用，选择 **"观看屏幕"**
2. 自动发现或手动输入信令服务器地址：
   - 应用会自动发现局域网内的 Publisher
   - 也可以手动输入：`ws://IP:PORT/ws`
3. 点击 **"刷新列表"** 获取可用的 Stream ID
4. 选择要观看的 Stream ID
5. 点击 **"订阅"** 开始观看
6. 支持 F11 全屏模式

## 🏗️ 项目结构

```
SnapScreen/
├── main.go                 # 程序入口
├── go.mod                  # Go 模块定义
├── internal/
│   ├── app/               # GUI 应用层
│   │   ├── gui.go         # 主窗口
│   │   ├── publisher_ui.go # Publisher UI
│   │   ├── viewer_ui.go   # Viewer UI
│   │   └── discovery.go   # 局域网发现
│   └── server/            # 信令服务器
│       ├── server.go      # 服务器核心
│       ├── client.go      # WebSocket 客户端
│       ├── router.go      # 消息路由
│       ├── stream.go      # 流管理
│       └── http.go        # HTTP 服务器
└── pkg/
    ├── client/            # WebRTC 客户端
    │   ├── common.go      # 公共类型和工具
    │   ├── publisher.go  # Publisher 实现
    │   └── viewer.go      # Viewer 实现
    ├── screen/            # 屏幕捕获
    │   └── capture.go
    ├── signal/            # 信令协议
    │   └── types.go
    ├── discovery/         # 发现服务
    │   └── discovery.go
    └── utils/             # 工具函数
        └── id_gen.go
```

## 🔧 技术栈

- **GUI 框架**：[Fyne](https://fyne.io/) v2.5.0
- **WebRTC**：[Pion WebRTC](https://github.com/pion/webrtc) v4.2.3
- **WebSocket**：[Gorilla WebSocket](https://github.com/gorilla/websocket) v1.5.3
- **屏幕捕获**：[kbinani/screenshot](https://github.com/kbinani/screenshot)

## 📝 配置说明

### Publisher 配置

- **FrameRate**：推流帧率，默认 30fps，上限 30fps
- **Width/Height**：输出分辨率，0 表示使用原始分辨率
- **区域捕获**：指定屏幕矩形区域（x, y, width, height）

### 信令服务器

- **内嵌模式**：Publisher 自动启动，端口由系统分配
- **外部模式**：手动指定 WebSocket 地址，格式：`ws://IP:PORT/ws`
- **默认地址**：`ws://127.0.0.1:8080/ws`

### 局域网发现

- **UDP 端口**：28901（固定）
- **广播间隔**：2 秒
- **自动发现**：Viewer 自动监听并发现 Publisher

## 🐛 故障排除

### 无法发现 Publisher

1. 确保 Publisher 和 Viewer 在同一局域网
2. 检查防火墙是否阻止 UDP 28901 端口
3. 尝试手动输入信令服务器地址

### 连接失败

1. 检查信令服务器地址是否正确
2. 确认 Stream ID 存在且已注册
3. 查看控制台错误信息

### 画面卡顿

1. 降低帧率设置（建议 20-25fps）
2. 降低输出分辨率
3. 检查网络带宽

## 📄 许可证

本项目采用 MIT 许可证。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📧 联系方式

如有问题或建议，请通过 Issue 反馈。

---

**注意**：本项目仅用于学习和研究目的，请勿用于商业用途。

