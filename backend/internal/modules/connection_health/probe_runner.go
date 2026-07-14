package connection_health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeTimeout 是单次真实探活请求的超时时间，任务书要求默认 10s。
const ProbeTimeout = 10 * time.Second

const defaultProbePrompt = "hi"

// ProbeRequest 是发起一次真实轻量探活所需的全部参数。UpstreamKey 只用于构造请求凭据，
// 探活结果（ProbeOutcome）绝不回填明文 key。
type ProbeRequest struct {
	BaseURL        string
	UpstreamKey    string
	ProviderFamily string
	ModelName      string
	MaxTokens      int
	ProbePrompt    string
}

// RealProbeRunner 按 provider family 构造最小请求，对上游 AI 端点发起一次性轻量调用。
// 不经过任何现有请求转发路径，独立的 http.Client，超时 10s。
type RealProbeRunner struct {
	client *http.Client
}

func NewRealProbeRunner() *RealProbeRunner {
	return &RealProbeRunner{client: &http.Client{Timeout: ProbeTimeout}}
}

// Probe 发起一次真实轻量探活，返回分类后的结果。err 只用于调用方感知调用本身是否被 ctx 取消，
// 正常的上游错误都归类进 ProbeOutcome.Result，不通过 error 返回。
func (r *RealProbeRunner) Probe(ctx context.Context, req ProbeRequest) ProbeOutcome {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1
	}
	prompt := strings.TrimSpace(req.ProbePrompt)
	if prompt == "" {
		prompt = defaultProbePrompt
	}

	httpReq, buildErr := buildProbeRequest(ctx, req, prompt, maxTokens)
	if buildErr != nil {
		return ProbeOutcome{Result: ResultInvalidResponse, Detail: redact(buildErr.Error(), req.UpstreamKey)}
	}

	started := time.Now()
	resp, err := r.client.Do(httpReq)
	latencyMs := int(time.Since(started).Milliseconds())
	if err != nil {
		return ProbeOutcome{Result: classifyTransportError(err), LatencyMs: latencyMs, Detail: redact(err.Error(), req.UpstreamKey)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return classifyHTTPResponse(resp.StatusCode, body, req.UpstreamKey, latencyMs)
}

// buildProbeRequest 统一走 OpenAI 兼容的 /v1/chat/completions 网关端点。
//
// 背景（对应整改任务书第 4 项）：real_connections.upstream_key 和
// upstream_site_id -> upstream_sites.base_url 代表的是已对接的 new-api / sub2api 网关凭据
// 和网关地址（一个可转发 Gemini/Anthropic/OpenAI 等多种模型的中转站点），不是 provider
// 官方凭据。new-api 和 sub2api 都以 OpenAI 兼容协议对外暴露 /v1/chat/completions，
// 内部按 model 名称路由到实际 provider。如果对这些网关直接打 Gemini
// generateContent / Anthropic messages 原生端点，网关大概率不认识这些路径，会导致
// Gemini/Anthropic 模型被系统性误判为失败，进而错误触发自动降级。
// providerFamily 目前只用于在 ModelName 为空时选择一个合理的默认模型名做探活，
// 不再影响实际请求的 endpoint/鉴权方式。
func buildProbeRequest(ctx context.Context, req ProbeRequest, prompt string, maxTokens int) (*http.Request, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	model := req.ModelName
	if model == "" {
		model = defaultModelForProvider(req.ProviderFamily)
	}

	endpoint := baseURL + "/v1/chat/completions"
	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
	}
	headers := map[string]string{"Authorization": "Bearer " + req.UpstreamKey}
	return newJSONRequest(ctx, http.MethodPost, endpoint, payload, headers)
}

func defaultModelForProvider(providerFamily string) string {
	switch providerFamily {
	case ProviderGemini:
		return "gemini-1.5-flash"
	case ProviderAnthropic:
		return "claude-3-haiku-20240307"
	default:
		return "gpt-4o-mini"
	}
}

func newJSONRequest(ctx context.Context, method string, endpoint string, payload any, headers map[string]string) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	return httpReq, nil
}

// classifyTransportError 归类连接层面的错误（超时、DNS、连接被拒等）为网络波动。
func classifyTransportError(err error) ResultKey {
	if errors.Is(err, context.DeadlineExceeded) {
		return ResultNetworkFluctuation
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return ResultNetworkFluctuation
	}
	return ResultNetworkFluctuation
}

// classifyHTTPResponse 按状态码和响应体归类为 7 种错误分类之一，或 ok。
func classifyHTTPResponse(status int, body []byte, upstreamKey string, latencyMs int) ProbeOutcome {
	detail := redact(truncate(string(body), 500), upstreamKey)

	switch {
	case status == http.StatusOK || status == http.StatusCreated:
		if !json.Valid(body) {
			return ProbeOutcome{Result: ResultInvalidResponse, LatencyMs: latencyMs, Detail: detail}
		}
		return ProbeOutcome{Result: ResultOK, LatencyMs: latencyMs, Detail: ""}

	case status == http.StatusTooManyRequests:
		return ProbeOutcome{Result: ResultRateLimited, LatencyMs: latencyMs, Detail: detail}

	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ProbeOutcome{Result: ResultAuth, LatencyMs: latencyMs, Detail: detail}

	case status == http.StatusNotFound:
		return ProbeOutcome{Result: ResultModelNotFound, LatencyMs: latencyMs, Detail: detail}

	case status >= 500:
		return ProbeOutcome{Result: ResultServerError, LatencyMs: latencyMs, Detail: detail}

	default:
		// 其余 4xx（参数错误等）无法安全归类为上游不可用，按响应无法解析处理，避免误判暂停。
		return ProbeOutcome{Result: ResultInvalidResponse, LatencyMs: latencyMs, Detail: detail}
	}
}

// redact 把探活凭据从错误信息/响应体中裁剪掉，事件和日志绝不落地明文 key。
func redact(s string, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "***")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
