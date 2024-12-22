package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"strings"

	"log"
	"path/filepath"

	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	g "github.com/AllenDang/giu"
	"github.com/disintegration/imaging"
	"github.com/harry1453/go-common-file-dialog/cfd"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
)

type AppConfig struct {
	SrcDir       string
	DstDir       string
	AllowColor   bool
	Threshold    int32
	WindowWidth  int
	WindowHeight int
}

var (
	wnd  *g.MasterWindow
	conf = AppConfig{
		SrcDir:       "",
		DstDir:       "",
		AllowColor:   false,
		Threshold:    20,
		WindowWidth:  800,
		WindowHeight: 600,
	}
)

var logBuffer = bytes.NewBufferString("")

var ImageSuffixes = [...]string{".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff"}

//go:embed winres/icon.png
var embedFs embed.FS

//go:embed winres/version.txt
var version string

func PickDir(title string, startDir string) (string, error) {
	pickFolderDialog, err := cfd.NewSelectFolderDialog(cfd.DialogConfig{
		Title: title,
		Role:  "ImageFolder",
	})
	if err != nil {
		return "", err
	}
	if err := pickFolderDialog.Show(); err != nil {
		return "", err
	}
	result, err := pickFolderDialog.GetResult()
	return result, err
}

func DirSelector(name string, target *string) *g.RowWidget {
	return g.Row(g.Button(name).OnClick(func() {
		filename, err := PickDir("选择"+name, *target)
		if err == nil {
			*target = filename
		} else {
			fmt.Println("ERR:", err)
		}
	}), g.InputText(target))
}

func isImageExt(ext string) bool {
	for _, b := range ImageSuffixes {
		if b == ext {
			return true
		}
	}
	return false
}

func getBorder(img *image.NRGBA, targetColor color.Color) int {
	bounds := img.Bounds()
	for x := 0; x < bounds.Max.X; x++ {
		for y := 0; y < bounds.Max.Y; y++ {
			c := img.At(x, y)
			if !isSimilarColor(c, targetColor) {
				// 出现非目标颜色, 返回列坐标
				// log.Printf("非目标颜色: %dx%d %v\n", x, y, c)
				return x
			}
		}
	}
	return bounds.Max.X
}

// Copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func sizeStr(img image.Image) string {
	bounds := img.Bounds()
	return fmt.Sprintf("%dx%d", bounds.Max.X, bounds.Max.Y)
}

func saveWebp(img image.Image, path string) error {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer output.Close()
	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 75)
	if err != nil {
		return err
	}
	err = webp.Encode(output, img, options)
	if err != nil {
		return err
	}
	return nil
}

// 判断两个颜色是否相似
func isSimilarColor(c1, c2 color.Color) bool {
	r1, g1, b1, _ := c1.RGBA()
	r2, g2, b2, _ := c2.RGBA()
	dr := int64(r1) - int64(r2)
	dg := int64(g1) - int64(g2)
	db := int64(b1) - int64(b2)

	dist := math.Sqrt(float64(dr*dr+dg*dg+db*db) / (3 * 256 * 256))
	// log.Printf("Dist: %v %v => %.3f", c1, c2, dist)
	return int32(dist) <= conf.Threshold
}

func handleImage(path string, dstPath string) bool {
	orignalImg, err := imaging.Open(path)
	if err != nil {
		log.Printf("打开来源图片失败 %s\n", err)
		return false
	}

	var targetColor color.Color
	if conf.AllowColor {
		targetColor = orignalImg.At(0, 0)
	} else {
		targetColor = color.NRGBA{0, 0, 0, 255}
	}
	log.Printf("原始图像: 大小=%s, 目标颜色=%v\n", sizeStr(orignalImg), targetColor)
	// 裁剪实现流程: 每次旋转90度并裁剪图像左侧部分, 重复4次
	img := imaging.Rotate90(orignalImg)
	trimmed := []int{}
	same := true
	for i := 0; i < 4; i++ {
		delta := getBorder(img, targetColor)
		trimmed = append(trimmed, delta)
		// imaging.Save(img, fmt.Sprintf("%s-%d.png", dstPath, i))
		if delta > 0 {
			curBounds := img.Bounds()
			img = imaging.Crop(img, image.Rect(delta, 0, curBounds.Max.X, curBounds.Max.Y))
			same = false
		}
		// imaging.Save(img, fmt.Sprintf("%s-%d-crop%d.png", dstPath, i, delta))
		if i < 3 {
			img = imaging.Rotate90(img)
		}
	}
	if same {
		// 未进行裁剪, 执行文件复制
		log.Printf("未进行裁剪, 执行文件复制\n")
		CopyFile(path, dstPath)
		return true
	}
	log.Printf("裁剪后大小=%s 裁剪区域=%v\n", sizeStr(img), trimmed)
	ext := strings.ToLower(filepath.Ext(dstPath))
	if ext == ".webp" {
		err = saveWebp(img, dstPath)
	} else {
		err = imaging.Save(img, dstPath)
	}
	if err != nil {
		log.Printf("保存图片失败 %s\n", err)
		return false
	}
	return true
}

func doTransfer() {
	logBuffer.Reset()
	files, err := os.ReadDir(conf.SrcDir)
	if err != nil {
		log.Printf("打开来源文件夹失败: %s", err)
		return
	}

	total := len(files)
	log.Printf("开始处理%d个文件: %s -> %s\n", total, conf.SrcDir, conf.DstDir)
	updated := 0
	for idx, f := range files {
		name := f.Name()
		ext := filepath.Ext(name)
		if !isImageExt(ext) {
			log.Println("跳过", name)
			continue
		}
		log.Println("开始处理", name)
		ret := handleImage(conf.SrcDir+"/"+name, conf.DstDir+"/"+name)
		if ret {
			updated++
		}
		log.Printf("处理完毕(%d/%d) %s\n", idx+1, total, name)
	}
	log.Printf("处理完毕, 共转换%d/%d个\n", updated, total)
}

func loop() {
	// 是否可以开始执行
	canTransfer := conf.SrcDir != "" && conf.DstDir != "" && conf.SrcDir != conf.DstDir

	// 修正阈值范围[0,200]
	if conf.Threshold < 0 {
		conf.Threshold = 0
	} else if conf.Threshold > 200 {
		conf.Threshold = 200
	}

	g.SingleWindow().Layout(
		g.Row(
			g.Label("图像去黑边工具 by AutumnSun"),
			g.Button("源码").OnClick(func() {
				g.OpenURL("https://github.com/AutumnSun1996/ImageTrim")
			}),
			g.Button("保存配置").OnClick(saveConfig),
			g.Button("加载配置").OnClick(loadConfig),
		),
		DirSelector("输入目录", &conf.SrcDir),
		DirSelector("输出目录", &conf.DstDir),
		g.Row(
			g.Row(g.Label("阈值[0,200]"), g.InputInt(&conf.Threshold).Size(40)),
			g.Checkbox("支持其他颜色", &conf.AllowColor),
			g.Button("开始转换").Disabled(!canTransfer).OnClick(doTransfer),
		),
		g.Label(logBuffer.String()),
	)
}

func loadConfigInit() {
	logBuffer.Reset()
	data, err := os.ReadFile("ImageTrim.json")
	if err != nil {
		return
	}
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return
	}
	log.Println("配置加载成功")
}

func loadConfig() {
	logBuffer.Reset()
	data, err := os.ReadFile("ImageTrim.json")
	if err != nil {
		log.Printf("读取配置文件失败, 将使用默认值: %s\n", err)
		return
	}
	err = json.Unmarshal(data, &conf)
	if err != nil {
		log.Printf("配置加载失败, 将使用默认值: %s\n", err)
		return
	}
	log.Println("配置加载成功")
}

func saveConfig() {
	logBuffer.Reset()
	// 记录窗口大小
	conf.WindowWidth, conf.WindowHeight = wnd.GetSize()
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		log.Printf("保存配置失败: %s\n", err)
		return
	}
	err = os.WriteFile("ImageTrim.json", data, 0644)
	if err != nil {
		log.Printf("保存配置文件失败: %s\n", err)
		return
	}
	log.Println("配置已保存到 ImageTrim.json")
}

func main() {
	log.SetOutput(logBuffer)
	// 首次加载配置
	loadConfigInit()
	wnd = g.NewMasterWindow("图像去黑边工具"+version, conf.WindowWidth, conf.WindowHeight, 0)
	// 设置icon
	f, err := embedFs.Open("icon.png")
	if err == nil {
		img, err := imaging.Decode(f)
		f.Close()
		if err == nil {
			wnd.SetIcon([]image.Image{img})
		}
	}
	wnd.Run(loop)
}
