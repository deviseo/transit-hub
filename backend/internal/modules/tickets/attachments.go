package tickets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// maxImageBytes：单张图片最大 5 MB。
const maxImageBytes = 5 << 20

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

// Delete 删除一张图片；已经不在了也算成功。
func (s *AttachmentStorage) Delete(name string) error {
	path, err := s.resolve(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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
