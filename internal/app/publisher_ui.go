package app

import (
	"fmt"
	"image"
	"log"
	"strconv"
	"strings"

	"snap-screen/internal/server"
	"snap-screen/pkg/client"
	"snap-screen/pkg/screen"
	"snap-screen/pkg/utils"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// 内嵌信令服务器 & 自动发现相关状态（仅 Publisher 使用）
var (
	embeddedSignalStop   func()
	embeddedSignalAddr   string
	embeddedDiscoverStop func()
)

// RunPublisherUI Publisher GUI
func RunPublisherUI(a fyne.App, w fyne.Window) {
	w.SetTitle("Publisher - 分享屏幕")

	streamIDEntry := widget.NewEntry()
	streamIDEntry.SetText(utils.GenID())
	streamIDEntry.SetPlaceHolder("自动生成或手动输入 streamID")

	signalEntry := widget.NewEntry()
	// 默认提示本地内嵌信令服务器，实际端口在开始分享时自动确定
	signalEntry.SetText("自动: 本机内嵌信令服务器")

	displays := screen.ListDisplays()
	screenOptions := make([]string, 0, len(displays))
	labelToDisplay := make(map[string]screen.DisplayInfo, len(displays))
	for _, d := range displays {
		screenOptions = append(screenOptions, d.Label)
		labelToDisplay[d.Label] = d
	}
	if len(screenOptions) == 0 {
		screenOptions = []string{"未检测到可用屏幕"}
	}
	screenSelect := widget.NewSelect(screenOptions, func(s string) {
		fmt.Println("选择屏幕:", s)
	})
	screenSelect.SetSelected(screenOptions[0])

	fpsEntry := widget.NewEntry()
	// 默认使用 30fps，避免在高分辨率屏幕上 CPU 占用过高导致卡顿
	fpsEntry.SetText("30")
	fpsEntry.SetPlaceHolder("帧率 (默认 30，建议 20~30)")

	widthEntry := widget.NewEntry()
	widthEntry.SetPlaceHolder("输出宽度（留空=原始）")
	heightEntry := widget.NewEntry()
	heightEntry.SetPlaceHolder("输出高度（留空=原始）")

	regionCheck := widget.NewCheck("启用区域捕获", nil)
	xEntry := widget.NewEntry()
	yEntry := widget.NewEntry()
	rwEntry := widget.NewEntry()
	rhEntry := widget.NewEntry()
	xEntry.SetPlaceHolder("x")
	yEntry.SetPlaceHolder("y")
	rwEntry.SetPlaceHolder("width")
	rhEntry.SetPlaceHolder("height")

	statusLabel := widget.NewLabel("状态: 未连接")
	statusDetail := widget.NewLabel("")

	var running bool
	// 确保仅在本窗口生命周期内启动一次内嵌服务器
	startEmbeddedIfNeeded := func() error {
		if embeddedSignalStop != nil {
			return nil
		}
		// 端口使用 0，由系统自动分配可用端口
		addr, stop, err := server.StartHTTPServer(":0")
		if err != nil {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("启动内嵌信令服务器失败: " + err.Error())
			return err
		}
		embeddedSignalStop = stop
		embeddedSignalAddr = addr

		// 从实际监听地址中解析端口，构造 WebSocket URL
		port := addr
		if idx := strings.LastIndex(addr, ":"); idx != -1 && idx+1 < len(addr) {
			port = addr[idx+1:]
		}
		wsURL := "ws://127.0.0.1:" + port + "/ws"
		signalEntry.SetText(wsURL)
		statusDetail.SetText("已启动内嵌信令服务器: " + wsURL)

		// 启动局域网 UDP 广播，让 Viewer 可以自动发现本 Publisher
		if embeddedDiscoverStop == nil {
			portInt, err := strconv.Atoi(port)
			if err != nil {
				return err
			}
			embeddedDiscoverStop = startDiscoveryBroadcast(portInt)
		}
		return nil
	}

	var startBtn *widget.Button
	var stopBtn *widget.Button
	startBtn = widget.NewButton("开始分享", func() {
		if running {
			return
		}

		// 先确保内嵌信令服务器就绪
		if err := startEmbeddedIfNeeded(); err != nil {
			return
		}

		if len(displays) == 0 {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("未找到可用屏幕")
			return
		}

		streamID := strings.TrimSpace(streamIDEntry.Text)
		if streamID == "" {
			streamID = utils.GenID()
			streamIDEntry.SetText(streamID)
		}

		screenInfo, ok := labelToDisplay[screenSelect.Selected]
		if !ok {
			screenInfo = displays[0]
		}

		fps, err := parseIntOrDefault(fpsEntry.Text, 60)
		if err != nil || fps <= 0 {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("帧率必须是正整数")
			return
		}
		width, err := parseIntOrDefault(widthEntry.Text, 0)
		if err != nil || width < 0 {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("宽度必须是非负整数")
			return
		}
		height, err := parseIntOrDefault(heightEntry.Text, 0)
		if err != nil || height < 0 {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("高度必须是非负整数")
			return
		}

		capture := screen.NewCapture(screenInfo.Index)
		if regionCheck.Checked {
			x, errX := parseIntOrDefault(xEntry.Text, 0)
			y, errY := parseIntOrDefault(yEntry.Text, 0)
			rw, errW := parseIntOrDefault(rwEntry.Text, 0)
			rh, errH := parseIntOrDefault(rhEntry.Text, 0)
			if errX != nil || errY != nil || errW != nil || errH != nil || rw <= 0 || rh <= 0 {
				statusLabel.SetText("状态: 错误")
				statusDetail.SetText("区域参数无效，需填写 x/y/width/height，且 width/height > 0")
				return
			}
			rect := image.Rect(x, y, x+rw, y+rh)
			capture.SetRegion(&rect)
		}

		cfg := client.PublisherConfig{
			SignalURL: strings.TrimSpace(signalEntry.Text),
			FrameRate: fps,
			Width:     width,
			Height:    height,
		}

		statusLabel.SetText("状态: 连接中")
		statusDetail.SetText("正在注册 stream: " + streamID)
		err = client.StartPublisher(streamID, capture, cfg, func(st client.PublisherStatus, detail string) {
			// 这里仅做轻量 UI 文本刷新，避免与 fyne 版本兼容问题。
			statusLabel.SetText("状态: " + string(st))
			statusDetail.SetText(detail)
		})
		if err != nil {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText(err.Error())
			return
		}

		running = true
		startBtn.Disable()
		stopBtn.Enable()
		streamIDEntry.Disable()
		screenSelect.Disable()
		signalEntry.Disable()
		fpsEntry.Disable()
		widthEntry.Disable()
		heightEntry.Disable()
		regionCheck.Disable()
		xEntry.Disable()
		yEntry.Disable()
		rwEntry.Disable()
		rhEntry.Disable()
	})

	stopBtn = widget.NewButton("停止分享", func() {
		log.Println("停止分享")
		client.StopPublisher()
		running = false
		statusLabel.SetText("状态: 已停止")
		statusDetail.SetText("已手动停止推流并释放资源")

		// 停止内嵌信令服务器（如需后续再次分享，将在下一次开始时重新启动）
		if embeddedSignalStop != nil {
			embeddedSignalStop()
			embeddedSignalStop = nil
			embeddedSignalAddr = ""
		}
		// 停止局域网广播
		if embeddedDiscoverStop != nil {
			embeddedDiscoverStop()
			embeddedDiscoverStop = nil
		}

		startBtn.Enable()
		stopBtn.Disable()
		streamIDEntry.Enable()
		screenSelect.Enable()
		signalEntry.Enable()
		fpsEntry.Enable()
		widthEntry.Enable()
		heightEntry.Enable()
		regionCheck.Enable()
		xEntry.Enable()
		yEntry.Enable()
		rwEntry.Enable()
		rhEntry.Enable()
	})
	stopBtn.Disable()

	content := container.NewVBox(
		widget.NewLabel("Publisher 模式"),
		widget.NewLabel("Stream ID"),
		streamIDEntry,
		widget.NewLabel("信令服务器"),
		signalEntry,
		widget.NewLabel("屏幕选择"),
		screenSelect,
		widget.NewLabel("帧率"),
		fpsEntry,
		widget.NewLabel("输出分辨率"),
		container.NewGridWithColumns(2, widthEntry, heightEntry),
		regionCheck,
		container.NewGridWithColumns(4, xEntry, yEntry, rwEntry, rhEntry),
		startBtn,
		stopBtn,
		statusLabel,
		statusDetail,
	)
	w.SetContent(content)
}

func parseIntOrDefault(text string, def int) (int, error) {
	v := strings.TrimSpace(text)
	if v == "" {
		return def, nil
	}
	return strconv.Atoi(v)
}
