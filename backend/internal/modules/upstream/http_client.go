package upstream

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// BrowserUserAgent 是所有上游 HTTP 请求统一使用的浏览器 User-Agent。
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

type HTTPClient struct {
	client *http.Client
}

type requestOptions struct {
	Method      string
	Body        any
	Cookie      string
	UserID      string
	AccessToken string
	TokenType   string
}

type jsonResponse struct {
	Payload any
	Header  http.Header
}

func NewHTTPClient(client *http.Client) *HTTPClient {
	return &HTTPClient{client: client}
}

func (c *HTTPClient) requestJSON(reqURL string, options requestOptions) (jsonResponse, error) {
	method := options.Method
	if method == "" {
		method = http.MethodGet
	}

	body, err := encodeBody(options.Body)
	if err != nil {
		return jsonResponse{}, err
	}

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return jsonResponse{}, newRequestError(ErrorInvalidURL, "")
	}
	req.Header.Set("Accept", "application/json")
	if options.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if options.AccessToken != "" {
		tokenType := options.TokenType
		if tokenType == "" {
			tokenType = "Bearer"
		}
		req.Header.Set("Authorization", tokenType+" "+options.AccessToken)
	}

	req.Header.Set("User-Agent", BrowserUserAgent)
	if options.Cookie != "" {
		req.Header.Set("Cookie", options.Cookie)
	}

	if options.UserID != "" {
		req.Header.Set("New-Api-User", options.UserID)
	}

	response, err := c.client.Do(req)
	if err != nil {
		log.Printf("[http-client] 请求失败 url=%s err=%v", reqURL, err)
		return jsonResponse{}, newRequestError(ErrorNetwork, "")
	}
	defer response.Body.Close()

	payload, err := parseJSON(response.Body, reqURL)
	if err != nil {
		return jsonResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		log.Printf("[http-client] 非 2xx 响应 url=%s status=%d", reqURL, response.StatusCode)
		// 保留原有全局行为（仅 401 归类为 ErrorAuth），但把真实 status code 一并带出，
		// 供需要区分 403 等细分状态的调用方判断，不改变其它调用方的既有语义。
		if response.StatusCode == http.StatusUnauthorized {
			return jsonResponse{}, newRequestErrorWithStatus(ErrorAuth, "", response.StatusCode)
		}
		return jsonResponse{}, newRequestErrorWithStatus(ErrorRequest, "", response.StatusCode)
	}
	return jsonResponse{Payload: payload, Header: response.Header}, nil
}

func encodeBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, newRequestError(ErrorInvalidResponse, "")
	}
	return bytes.NewReader(encoded), nil
}

func parseJSON(reader io.Reader, reqURL string) (any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("[http-client] 读取响应体失败 url=%s err=%v", reqURL, err)
		return nil, newRequestError(ErrorInvalidResponse, "")
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		preview := string(data)
		if len(preview) > 500 {
			preview = preview[:500] + "...(truncated)"
		}
		log.Printf("[http-client] JSON 解析失败 url=%s len=%d preview=%s", reqURL, len(data), preview)
		return nil, newRequestError(ErrorInvalidResponse, "")
	}
	return payload, nil
}
