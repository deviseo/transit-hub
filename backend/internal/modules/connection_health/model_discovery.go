package connection_health

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"transithub/backend/internal/modules/upstream"
)

// 本文件实现手动一次性探活弹窗打开时的 server-only 模型发现：用当前 admin session 临时解析
// 出的 base_url + key 请求上游 OpenAI 兼容 /v1/models，只把安全字段透出前端。
// 与 probe_runner.go 的真实探活请求相互独立（发现模型不消耗探活预算、不落库）。

// modelDiscoveryTimeout 与真实探活的 ProbeTimeout 保持一致的保守超时。
const modelDiscoveryTimeout = ProbeTimeout

// DiscoveredModel 是模型发现接口的对外展示字段，绝不包含 base_url/key 等敏感信息。
type DiscoveredModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	OwnedBy        string `json:"ownedBy"`
	ProviderFamily string `json:"providerFamily"`
}

// ModelDiscoveryRunner 独立于 RealProbeRunner，专门请求 /v1/models 列表。
type ModelDiscoveryRunner struct {
	client *http.Client
}

func NewModelDiscoveryRunner() *ModelDiscoveryRunner {
	return &ModelDiscoveryRunner{client: &http.Client{Timeout: modelDiscoveryTimeout}}
}

// openAIModelListResponse 兼容常见 OpenAI 结构：{"data":[{"id":"...","owned_by":"..."}]}。
type openAIModelListResponse struct {
	Data []struct {
		ID      string `json:"id"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// ListModels 请求 {baseURL}/v1/models 并解析出去空、去重、按名称排序后的模型列表。
// 上游不可达/非 2xx -> ErrorModelListUnavailable；返回体不是可识别的 OpenAI 兼容结构 ->
// ErrorModelListInvalid。两种错误都不携带上游原始报文，避免把上游异常明细泄露给前端。
func (r *ModelDiscoveryRunner) ListModels(ctx context.Context, baseURL string, key string) ([]DiscoveredModel, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, requestError(ErrorModelListUnavailable)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, requestError(ErrorModelListUnavailable)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, requestError(ErrorModelListUnavailable)
	}

	var payload openAIModelListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, requestError(ErrorModelListInvalid)
	}

	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]DiscoveredModel, 0, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, DiscoveredModel{ID: id, Name: id, OwnedBy: strings.TrimSpace(m.OwnedBy)})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// DiscoverTargetModels 是手动探活弹窗打开时调用的服务方法：重新解析 targetId 归属 + 凭据，
// 拿到该 target 当前真实可用的模型列表，模型池不依赖任何探活策略配置。
func (s *Service) DiscoverTargetModels(ctx context.Context, userID string, targetID string) ([]DiscoveredModel, error) {
	session, _, account, _, err := s.resolveManualTarget(ctx, userID, targetID)
	if err != nil {
		return nil, err
	}
	cred, err := s.platformGroups.ResolveProbeCredential(session, account)
	if err != nil {
		return nil, requestError(reasonToErrorKey(upstream.ProbeCredentialReason(err)))
	}
	return s.modelDiscovery.ListModels(ctx, cred.BaseURL, cred.Key)
}
