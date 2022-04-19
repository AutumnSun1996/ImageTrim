package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"io"
	"os"
	"strings"

	"io/ioutil"
	"log"
	"path/filepath"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	g "github.com/AllenDang/giu"
	"github.com/disintegration/imaging"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
	"github.com/sqweek/dialog"
)

var (
	threshold int32  = 20
	srcDir    string = ""
	dstDir    string = ""
)
var logBuffer = bytes.NewBufferString("")

var ImageSuffixes = [...]string{".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff"}

//go:embed winres/icon.png
var embedFs embed.FS

//go:embed winres/version.txt
var version string

func SelectSrcFile() {
	filename, err := dialog.Directory().Browse()
	if err == nil {
		srcDir = filename
	} else {
		fmt.Println("ERR:", err)
	}
}

func DirSelector(name string, target *string) *g.RowWidget {
	return g.Row(g.Button(name).OnClick(func() {
		filename, err := dialog.Directory().Title("选择" + name).SetStartDir(*target).Browse()
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

func rotate(matrix [][]int) {
	row := len(matrix)
	if row <= 0 {
		return
	}
	column := len(matrix[0])

	for i := 0; i < row; i++ {
		for j := i + 1; j < column; j++ {
			tmp := matrix[i][j]
			matrix[i][j] = matrix[j][i]
			matrix[j][i] = tmp
		}
	}

	halfColumn := column / 2
	for i := 0; i < row; i++ {
		for j := 0; j < halfColumn; j++ {
			tmp := matrix[i][j]
			matrix[i][j] = matrix[i][column-j-1]
			matrix[i][column-j-1] = tmp
		}
	}
}

func getBorder(img *image.NRGBA, tol uint8) int {
	bounds := img.Bounds()
	for x := 0; x < bounds.Max.X; x++ {
		for y := 0; y < bounds.Max.Y; y++ {
			c := img.NRGBAAt(x, y)
			if c.A > tol && (c.R > tol || c.G > tol || c.B > tol) {
				// 出现非0像素, 返回列坐标
				// log.Printf("非0像素: %dx%d (tol=%d) %v\n", x, y, tol, c)
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

func showSize(name string, img image.Image) {
	bounds := img.Bounds()
	log.Printf("%s大小: %dx%d\n", name, bounds.Max.X, bounds.Max.Y)
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

func handleImage(path string, dstPath string) bool {
	orignalImg, err := imaging.Open(path)
	if err != nil {
		log.Printf("打开来源图片失败 %s\n", err)
		return false
	}

	showSize("原始图像", orignalImg)
	// 裁剪实现流程: 每次旋转90度并裁剪图像左侧部分, 重复4次
	img := imaging.Rotate90(orignalImg)
	trimmed := []int{}
	same := true
	for i := 0; i < 4; i++ {
		delta := getBorder(img, uint8(threshold))
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
	log.Printf("裁剪: %v\n", trimmed)
	showSize("裁剪后图像", img)
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
	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		log.Printf("打开来源文件夹失败: %s", err)
		return
	}

	total := len(files)
	log.Printf("开始转换: %s -> %s\n", srcDir, dstDir)
	log.Printf("共%d个文件\n", total)
	updated := 0
	for idx, f := range files {
		name := f.Name()
		ext := filepath.Ext(name)
		if !isImageExt(ext) {
			log.Println("跳过", name)
			continue
		}
		log.Println("开始处理", name)
		ret := handleImage(srcDir+"/"+name, dstDir+"/"+name)
		if ret {
			updated++
		}
		log.Printf("处理完毕(%d/%d) %s\n", idx+1, total, name)
	}
	log.Printf("处理完毕, 共转换%d/%d个\n", updated, total)
}

func loop() {
	// 是否可以开始执行
	canTransfer := srcDir != "" && dstDir != "" && srcDir != dstDir

	// 修正阈值范围[0,200]
	if threshold < 0 {
		threshold = 0
	} else if threshold > 200 {
		threshold = 200
	}

	g.SingleWindow().Layout(
		g.Row(
			g.Label("图像去黑边工具 by AutumnSun"),
			g.Button("源码").OnClick(func() {
				g.OpenURL("https://github.com/AutumnSun1996/ImageTrim")
			}),
		),
		DirSelector("输入目录", &srcDir),
		DirSelector("输出目录", &dstDir),
		g.Row(
			g.Row(g.Label("阈值[0,200]"), g.InputInt(&threshold).Size(40)),
			g.Button("开始转换").Disabled(!canTransfer).OnClick(doTransfer),
		),
		g.Label(logBuffer.String()),
	)
}

func main() {
	log.SetOutput(logBuffer)
	srcDir, _ = filepath.Abs("./test-in")
	dstDir, _ = filepath.Abs("./test-out")
	wnd := g.NewMasterWindow("图像去黑边工具"+version, 800, 600, 0)
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
