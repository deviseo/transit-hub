package tickets

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// tinyWebP 是一张 1×1 的无损 WebP。
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func thumbnailSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		t.Fatalf("thumbnail should be a JPEG, got %q %v", format, err)
	}
	return config.Width, config.Height
}

func newStorage(t *testing.T) *AttachmentStorage {
	t.Helper()
	storage, err := NewAttachmentStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return storage
}

func TestThumbnailsAreSmallJPEGsCachedNextToTheImage(t *testing.T) {
	storage := newStorage(t)
	name, err := storage.Save("image/png", encodePNG(t, 1200, 800))
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := storage.Thumbnail(name)
	if err != nil {
		t.Fatal(err)
	}
	if width, height := thumbnailSize(t, thumb); width != thumbnailSide || height != 320 {
		t.Fatalf("thumbnail size %dx%d", width, height)
	}
	cached := filepath.Join(storage.baseDir, name+thumbnailSuffix)
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("thumbnail not cached: %v", err)
	}
	if again, err := storage.Thumbnail(name); err != nil || !bytes.Equal(again, thumb) {
		t.Fatalf("cached thumbnail differs: %v", err)
	}

	// 删掉图片时缩略图一起删，目录里不留临时文件。
	if err := storage.Delete(name); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(storage.baseDir); len(entries) != 0 {
		t.Fatalf("files left behind: %v", entries)
	}
}

func TestThumbnailsOfSmallGIFAndWebPImagesAreNotEnlarged(t *testing.T) {
	storage := newStorage(t)
	var animation bytes.Buffer
	frame := image.NewPaletted(image.Rect(0, 0, 40, 30), []color.Color{color.White, color.Black})
	if err := gif.Encode(&animation, frame, nil); err != nil {
		t.Fatal(err)
	}
	name, _ := storage.Save("image/gif", animation.Bytes())
	thumb, err := storage.Thumbnail(name)
	if err != nil {
		t.Fatal(err)
	}
	if width, height := thumbnailSize(t, thumb); width != 40 || height != 30 {
		t.Fatalf("gif thumbnail %dx%d", width, height)
	}

	webp, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatal(err)
	}
	name, _ = storage.Save("image/webp", webp)
	thumb, err = storage.Thumbnail(name)
	if err != nil {
		t.Fatalf("webp thumbnail: %v", err)
	}
	if width, height := thumbnailSize(t, thumb); width != 1 || height != 1 {
		t.Fatalf("webp thumbnail %dx%d", width, height)
	}
}

// pngHeader 是一个只有文件头的 PNG，声称图片有 width×height 那么大。
func pngHeader(width, height uint32) []byte {
	var chunk bytes.Buffer
	chunk.WriteString("IHDR")
	_ = binary.Write(&chunk, binary.BigEndian, width)
	_ = binary.Write(&chunk, binary.BigEndian, height)
	chunk.Write([]byte{8, 6, 0, 0, 0})
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&out, binary.BigEndian, uint32(13))
	out.Write(chunk.Bytes())
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()))
	return out.Bytes()
}

func TestHugeOrBrokenImagesGetNoThumbnail(t *testing.T) {
	storage := newStorage(t)
	// 声称 20000×20000 的图片：只读尺寸，不解码（防止占满内存），调用方改用原图。
	huge, _ := storage.Save("image/png", pngHeader(20000, 20000))
	if _, err := storage.Thumbnail(huge); err == nil {
		t.Fatal("huge images must not be decoded")
	}
	broken, _ := storage.Save("image/png", append([]byte("\x89PNG\r\n\x1a\n"), "not really a png"...))
	if _, err := storage.Thumbnail(broken); err == nil {
		t.Fatal("broken images have no thumbnail")
	}
	for _, name := range []string{huge, broken} {
		if _, err := os.Stat(filepath.Join(storage.baseDir, name+thumbnailSuffix)); !os.IsNotExist(err) {
			t.Fatalf("no thumbnail file should be written for %s", name)
		}
	}
}
