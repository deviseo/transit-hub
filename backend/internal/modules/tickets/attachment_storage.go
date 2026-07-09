package tickets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// AttachmentStorage 把工单图片附件保存在本地磁盘目录，不落库、不暴露成公开静态资源
// （目录不应指向前端 PUBLIC_DIR）。文件名完全由服务端生成，绝不使用用户上传的原始文件名，
// 从根本上避免路径穿越、文件名冲突或覆盖。
type AttachmentStorage struct {
	baseDir string
}

// NewAttachmentStorage 创建存储实例并确保目录存在（目录不存在时自动创建）。
func NewAttachmentStorage(baseDir string) (*AttachmentStorage, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	return &AttachmentStorage{baseDir: absBase}, nil
}

// Save 把图片数据写入一个服务端生成的新文件，返回相对 baseDir 的存储路径（落库用，
// 是一个不含目录分隔符的纯文件名，天然不可能携带 ".." 穿越片段）。
func (s *AttachmentStorage) Save(contentType string, data []byte) (string, error) {
	name, err := randomAttachmentFilename(imageExtensionFor(contentType))
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(s.baseDir, name)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// Read 按存储路径读取图片数据。resolve 会拒绝任何试图跳出 baseDir 的路径，
// 这里是纵深防御：正常情况下 storagePath 只会是 Save 生成的纯文件名。
func (s *AttachmentStorage) Read(storagePath string) ([]byte, error) {
	fullPath, err := s.resolve(storagePath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

// Delete 删除一个已保存的文件；文件已不存在时视为成功（幂等），用于事务失败后的补偿清理。
func (s *AttachmentStorage) Delete(storagePath string) error {
	fullPath, err := s.resolve(storagePath)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *AttachmentStorage) resolve(storagePath string) (string, error) {
	// 只取文件名部分，忽略调用方可能传入的任何目录片段（包括 ".."），
	// 保证最终路径永远落在 baseDir 内。
	name := filepath.Base(filepath.Clean(storagePath))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "", errors.New("invalid attachment storage path")
	}
	fullPath := filepath.Join(s.baseDir, name)
	if !strings.HasPrefix(fullPath, s.baseDir+string(os.PathSeparator)) {
		return "", errors.New("invalid attachment storage path")
	}
	return fullPath, nil
}

func randomAttachmentFilename(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("generate attachment filename")
	}
	return hex.EncodeToString(buf) + ext, nil
}
