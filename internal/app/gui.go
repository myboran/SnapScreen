package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// RunMainGUI 启动模式选择 GUI
func RunMainGUI() {
	a := app.New()
	w := a.NewWindow("屏幕共享 App")
	w.Resize(fyne.NewSize(1000, 600))

	modeLabel := widget.NewLabel("选择模式:")
	publisherBtn := widget.NewButton("分享屏幕", func() {
		RunPublisherUI(a, w)
	})
	viewerBtn := widget.NewButton("观看屏幕", func() {
		RunViewerUI(a, w)
	})

	w.SetContent(container.NewVBox(
		modeLabel,
		publisherBtn,
		viewerBtn,
	))
	w.ShowAndRun()
}
