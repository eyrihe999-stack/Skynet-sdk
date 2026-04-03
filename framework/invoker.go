package framework

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Invoker 提供在 Skill Handler 中调用其他 Agent Skill 的能力。
// 通过 HTTP 调用 Platform 的 /api/v1/invoke 端点，自动携带认证信息和 call_chain。
type Invoker struct {
	registryURL string
	apiKey      string
	agentID     string   // 当前 Agent 的 ID，用于 call_chain
	callChain   []string // 当前调用链
}

// InvokeResult 是远程 Skill 调用的结果。
type InvokeResult struct {
	TaskID string         `json:"task_id"`
	Status string         `json:"status"`
	Output map[string]any `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// Invoke 调用网络上另一个 Agent 的 Skill。
//
// 使用示例：
//
//	result, err := ctx.Invoke("legal-bot", "review_contract", map[string]any{
//	    "contract_text": "...",
//	})
func (inv *Invoker) Invoke(targetAgent, skill string, input any) (*InvokeResult, error) {
	// 构造 call_chain：当前链 + 自己的 agent_id
	chain := make([]string, len(inv.callChain))
	copy(chain, inv.callChain)
	chain = append(chain, inv.agentID)

	body := map[string]any{
		"target_agent": targetAgent,
		"skill":        skill,
		"input":        input,
		"timeout_ms":   30000,
		"call_chain":   chain,
	}
	bodyBytes, _ := json.Marshal(body)

	url := inv.registryURL + "/api/v1/invoke"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", inv.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("invoke request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// 解析外层 API 响应
	var apiResp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return nil, fmt.Errorf("invoke failed: %s", apiResp.Message)
	}

	var result InvokeResult
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("parse invoke result failed: %w", err)
	}

	if result.Status == "failed" {
		return nil, fmt.Errorf("skill '%s' on '%s' failed: %s", skill, targetAgent, result.Error)
	}

	return &result, nil
}
