package screen

import (
	"image"
	"log"
	"strconv"

	"github.com/kbinani/screenshot"
	xdraw "golang.org/x/image/draw"
)

// DisplayInfo 描述可用屏幕信息
type DisplayInfo struct {
	Index  int
	Bounds image.Rectangle
	Label  string
}

type Capture struct {
	screenIndex int
	region      *image.Rectangle
}

// NewCapture 创建屏幕捕获实例
func NewCapture(index int) *Capture {
	return &Capture{screenIndex: index}
}

// SetRegion 设置捕获区域（相对屏幕左上角），nil 表示整个屏幕
func (c *Capture) SetRegion(region *image.Rectangle) {
	c.region = region
}

// ListDisplays 列出所有可用屏幕
func ListDisplays() []DisplayInfo {
	n := screenshot.NumActiveDisplays()
	out := make([]DisplayInfo, 0, n)
	for i := 0; i < n; i++ {
		b := screenshot.GetDisplayBounds(i)
		out = append(out, DisplayInfo{
			Index:  i,
			Bounds: b,
			Label:  displayLabel(i, b),
		})
	}
	return out
}

// CaptureFrame 捕获当前屏幕帧
func (c *Capture) CaptureFrame() *image.RGBA {
	n := screenshot.NumActiveDisplays()
	if c.screenIndex >= n {
		log.Printf("屏幕索引 %d 超出范围", c.screenIndex)
		return nil
	}
	bounds := screenshot.GetDisplayBounds(c.screenIndex)
	targetBounds := bounds
	if c.region != nil {
		crop := c.region.Add(bounds.Min)
		crop = crop.Intersect(bounds)
		if crop.Empty() {
			log.Println("捕获区域无效：超出屏幕范围")
			return nil
		}
		targetBounds = crop
	}

	img, err := screenshot.CaptureRect(targetBounds)
	if err != nil {
		log.Println("捕获屏幕失败:", err)
		return nil
	}
	return img
}

// CaptureFrameSized 捕获当前屏幕帧，并缩放到目标分辨率
func (c *Capture) CaptureFrameSized(width, height int) *image.RGBA {
	frame := c.CaptureFrame()
	if frame == nil {
		return nil
	}
	if width <= 0 || height <= 0 {
		return frame
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	// 使用 ApproxBiLinear 相比 CatmullRom 更快，画质对屏幕共享场景足够
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), frame, frame.Bounds(), xdraw.Over, nil)
	return dst
}

func displayLabel(index int, bounds image.Rectangle) string {
	w := bounds.Dx()
	h := bounds.Dy()
	return "屏幕" + strconv.Itoa(index) + " (" + strconv.Itoa(w) + "x" + strconv.Itoa(h) + ")"
}
