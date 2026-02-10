package app

import (
	"fmt"
	"log"
	"strings"

	"snap-screen/pkg/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// RunViewerUI Viewer GUI
func RunViewerUI(a fyne.App, w fyne.Window) {
	w.SetTitle("Viewer - 观看屏幕")

	signalEntry := widget.NewEntry()
	signalEntry.SetText("自动选择或手动填写信令服务器 ws://ip:port/ws")

	// 自动发现到的 Publisher 列表：label -> wsURL
	type discoveredPublisher struct {
		WSURL string
	}
	discovered := map[string]*discoveredPublisher{}
	publisherSelect := widget.NewSelect([]string{}, func(label string) {
		if p, ok := discovered[label]; ok {
			signalEntry.SetText(p.WSURL)
		}
	})
	publisherSelect.PlaceHolder = "自动发现到的 Publisher（可选）"

	streamSelect := widget.NewSelect([]string{}, nil)
	streamSelect.PlaceHolder = "请选择要订阅的 Stream ID"

	statusLabel := widget.NewLabel("状态: 未连接")
	statusDetail := widget.NewLabel("")

	// 启动 UDP 监听，自动发现局域网内的 Publisher
	startDiscoveryListener(func(ip string, port int) {
		wsURL := fmt.Sprintf("ws://%s:%d/ws", ip, port)
		label := fmt.Sprintf("%s:%d", ip, port)

		if _, ok := discovered[label]; !ok {
			discovered[label] = &discoveredPublisher{WSURL: wsURL}
		}

		opts := make([]string, 0, len(discovered))
		for k := range discovered {
			opts = append(opts, k)
		}
		publisherSelect.Options = opts
		publisherSelect.Refresh()
	})

	refreshBtn := widget.NewButton("刷新列表", func() {
		ids, err := client.FetchStreamList(strings.TrimSpace(signalEntry.Text))
		if err != nil {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("拉取流列表失败: " + err.Error())
			return
		}
		if len(ids) == 0 {
			statusLabel.SetText("状态: 提示")
			statusDetail.SetText("当前没有可用的流")
		} else {
			statusLabel.SetText("状态: 就绪")
			statusDetail.SetText("请选择要订阅的 Stream ID")
		}
		streamSelect.Options = ids
		if len(ids) > 0 {
			streamSelect.SetSelected(ids[0])
		} else {
			streamSelect.SetSelected("")
		}
	})

	subBtn := widget.NewButton("订阅", func() {
		streamID := streamSelect.Selected
		if streamID == "" {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("请先选择一个 Stream ID")
			return
		}
		log.Println("订阅流:", streamID)

		// 为每次订阅单独弹出一个窗口用于显示画面
		viewWin := a.NewWindow("Viewer - " + streamID)
		viewWin.Resize(fyne.NewSize(960, 540))

		img := canvas.NewImageFromImage(nil)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(800, 450))

		// 支持 F11 切换真正全屏
		viewWin.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
			if ev.Name == fyne.KeyF11 {
				viewWin.SetFullScreen(!viewWin.FullScreen())
			}
		})

		viewWin.SetContent(img)
		viewWin.Show()

		if err := client.StartViewer(streamID, img, strings.TrimSpace(signalEntry.Text)); err != nil {
			statusLabel.SetText("状态: 错误")
			statusDetail.SetText("订阅失败: " + err.Error())
			return
		}
		statusLabel.SetText("状态: 已订阅")
		statusDetail.SetText("正在接收远程画面")
	})
	unsubBtn := widget.NewButton("取消订阅", func() {
		log.Println("取消订阅")
		client.StopViewer()
		statusLabel.SetText("状态: 已取消")
		statusDetail.SetText("已取消订阅")
	})

	content := container.NewVBox(
		widget.NewLabel("Viewer 模式"),
		widget.NewLabel("自动发现的 Publisher"),
		publisherSelect,
		widget.NewLabel("信令服务器"),
		signalEntry,
		container.NewGridWithColumns(2, streamSelect, refreshBtn),
		statusLabel,
		statusDetail,
		subBtn,
		unsubBtn,
	)
	w.SetContent(content)
}
