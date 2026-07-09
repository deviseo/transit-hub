package tickets

import (
	"net/http"
	"strings"
)

// maxImageSizeBytes 单张图片大小上限（5MB），拒绝超限文件。
const maxImageSizeBytes = 5 * 1024 * 1024

// allowedImageContentTypes 只允许这四种图片格式；键是规范化后的 content-type，值是落盘用的扩展名。
var allowedImageContentTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

func isAllowedImageContentType(contentType string) bool {
	_, ok := allowedImageContentTypes[normalizeContentType(contentType)]
	return ok
}

func imageExtensionFor(contentType string) string {
	return allowedImageContentTypes[normalizeContentType(contentType)]
}

func normalizeContentType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(contentType))
}

// sniffImageContentType 用标准库 net/http 的魔数嗅探得出真实 content-type，不信任客户端在
// multipart 部分头里声明的 Content-Type（可被伪造）。只有嗅探结果本身也在允许列表内才通过，
// 嗅探得到的类型同时作为落库和后续下载响应头使用的权威 content-type。
func sniffImageContentType(data []byte) (string, bool) {
	sniffed := normalizeContentType(http.DetectContentType(data))
	// DetectContentType 对某些 jpeg/webp 变体可能带参数（如 "image/jpeg; charset=binary" 的情况
	// 实际不会出现，但仍按分号裁剪一次，确保命中 map 里的裸类型）。
	if idx := strings.Index(sniffed, ";"); idx >= 0 {
		sniffed = strings.TrimSpace(sniffed[:idx])
	}
	_, ok := allowedImageContentTypes[sniffed]
	return sniffed, ok
}

// validateAttachmentUploads 校验一批待上传的图片：数量不超过 maxImages，且每张图片非空、
// 大小不超限、嗅探出的真实 content-type 在允许列表内。校验通过后返回的切片里 ContentType
// 已替换为嗅探结果，调用方（Service）应使用返回值而不是原始输入继续处理。
func validateAttachmentUploads(uploads []AttachmentUpload, maxImages int) ([]AttachmentUpload, error) {
	if len(uploads) == 0 {
		return uploads, nil
	}
	if len(uploads) > maxImages {
		return nil, requestError(ErrorEmbedTooManyImages)
	}
	validated := make([]AttachmentUpload, 0, len(uploads))
	for _, upload := range uploads {
		if len(upload.Data) == 0 {
			return nil, requestError(ErrorEmbedEmptyImage)
		}
		if len(upload.Data) > maxImageSizeBytes {
			return nil, requestError(ErrorEmbedImageTooLarge)
		}
		sniffed, ok := sniffImageContentType(upload.Data)
		if !ok {
			return nil, requestError(ErrorEmbedInvalidImageType)
		}
		validated = append(validated, AttachmentUpload{
			OriginalName: upload.OriginalName,
			ContentType:  sniffed,
			Data:         upload.Data,
		})
	}
	return validated, nil
}
