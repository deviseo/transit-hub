package tickets

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/draw"
	_ "image/gif" // 注册解码器：缩略图要解码 GIF、JPEG、PNG、WebP
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// maxImageBytes：单张图片最大 5 MB。
const maxImageBytes = 5 << 20

const (
	// thumbnailSide：缩略图最长边的像素。对话里的缩略图最大 128×80（两倍屏 256×160），留些余量。
	thumbnailSide = 480
	// thumbnailSuffix：缩略图和原图放在一起，文件名是原图的文件名加上它。
	thumbnailSuffix = ".thumb.jpg"
	// maxThumbnailPixels：超过这么多像素的图片不做缩略图（解码太占内存），直接用原图。
	maxThumbnailPixels = 40_000_000
)

// errNoThumbnail：这张图片做不了缩略图，用原图。
var errNoThumbnail = errors.New("no thumbnail for this image")

// thumbnailSlots 限制同时生成缩略图的数量（解码大图很占内存）。
var thumbnailSlots = make(chan struct{}, 2)

// imageExtensions 是允许的图片类型（按文件内容判断）和保存时用的扩展名。
var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// AttachmentStorage 把图片存在数据目录的 ticket-uploads 里（不在前端静态目录下，只能通过鉴权的接口下载）。
// 文件名由服务端随机生成，不用上传时的文件名。
type AttachmentStorage struct {
	baseDir string
}

// NewAttachmentStorage 创建存储目录。
func NewAttachmentStorage(baseDir string) (*AttachmentStorage, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	return &AttachmentStorage{baseDir: absolute}, nil
}

// Save 保存一张图片，返回文件名（存进数据库）。
func (s *AttachmentStorage) Save(contentType string, data []byte) (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	name := hex.EncodeToString(buffer) + imageExtensions[contentType]
	if err := os.WriteFile(filepath.Join(s.baseDir, name), data, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// Read 读取一张图片。
func (s *AttachmentStorage) Read(name string) ([]byte, error) {
	path, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Thumbnail 返回图片的缩略图（JPEG，最长边 thumbnailSide）。第一次请求时生成，存成原图文件名加
// thumbnailSuffix，之后直接读。做不了缩略图（图片太大、文件损坏）时返回错误，调用方改用原图。
func (s *AttachmentStorage) Thumbnail(name string) ([]byte, error) {
	path, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	thumbPath := path + thumbnailSuffix
	if data, err := os.ReadFile(thumbPath); err == nil {
		return data, nil
	}
	thumbnailSlots <- struct{}{}
	defer func() { <-thumbnailSlots }()
	// 排队的时候，别的请求可能已经生成好了。
	if data, err := os.ReadFile(thumbPath); err == nil {
		return data, nil
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data, err := makeThumbnail(original)
	if err != nil {
		return nil, err
	}
	// 先写临时文件再改名：同时来的请求不会读到写了一半的文件。存不下也不影响这次返回。
	if temp, err := os.CreateTemp(s.baseDir, ".thumb-*"); err == nil {
		_, writeErr := temp.Write(data)
		closeErr := temp.Close()
		if writeErr != nil || closeErr != nil || os.Rename(temp.Name(), thumbPath) != nil {
			_ = os.Remove(temp.Name())
		}
	}
	return data, nil
}

// makeThumbnail 把图片缩小到最长边 thumbnailSide（小图不放大），转成 JPEG；透明的地方铺白色。
// GIF 只取第一帧。先读尺寸，太大的图片不解码。
func makeThumbnail(original []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > maxThumbnailPixels {
		return nil, errNoThumbnail
	}
	source, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if longest := max(width, height); longest > thumbnailSide {
		width = max(1, width*thumbnailSide/longest)
		height = max(1, height*thumbnailSide/longest)
	}
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(target, target.Bounds(), image.White, image.Point{}, draw.Src)
	xdraw.BiLinear.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, target, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Delete 删除一张图片和它的缩略图；已经不在了也算成功。
func (s *AttachmentStorage) Delete(name string) error {
	path, err := s.resolve(name)
	if err != nil {
		return err
	}
	for _, file := range []string{path, path + thumbnailSuffix} {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// resolve 只取文件名部分，保证路径落在存储目录里。
func (s *AttachmentStorage) resolve(name string) (string, error) {
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", errors.New("invalid attachment name")
	}
	path := filepath.Join(s.baseDir, base)
	if !strings.HasPrefix(path, s.baseDir+string(os.PathSeparator)) {
		return "", errors.New("invalid attachment name")
	}
	return path, nil
}

// validateUploads 检查一批图片：数量、大小，按文件内容判断类型（不信任上传时声明的类型）。
// 返回的图片 ContentType 换成了判断出的真实类型。
func validateUploads(uploads []AttachmentUpload) ([]AttachmentUpload, error) {
	if len(uploads) > maxImagesPerMessage {
		return nil, requestError(ErrorTooManyImages)
	}
	validated := make([]AttachmentUpload, 0, len(uploads))
	for _, upload := range uploads {
		if len(upload.Data) == 0 {
			return nil, requestError(ErrorInvalidImage)
		}
		if len(upload.Data) > maxImageBytes {
			return nil, requestError(ErrorImageTooLarge)
		}
		sniffed := strings.ToLower(strings.TrimSpace(http.DetectContentType(upload.Data)))
		if index := strings.Index(sniffed, ";"); index >= 0 {
			sniffed = strings.TrimSpace(sniffed[:index])
		}
		if _, ok := imageExtensions[sniffed]; !ok {
			return nil, requestError(ErrorInvalidImage)
		}
		name := strings.TrimSpace(filepath.Base(upload.OriginalName))
		if name == "" || name == "." || len(name) > 200 {
			name = "image" + imageExtensions[sniffed]
		}
		validated = append(validated, AttachmentUpload{OriginalName: name, ContentType: sniffed, Data: upload.Data})
	}
	return validated, nil
}
