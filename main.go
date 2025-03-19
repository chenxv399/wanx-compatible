package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	port          = flag.String("port", "8080", "Server port")
	openaiKey     = flag.String("openai-key", "", "OpenAI-compatible API key")
	dashscopeKey  = flag.String("dashscope-key", "", "DashScope API key")
	allowedModels = map[string]bool{
		"wanx2.0-t2i-turbo": true,
		"wanx2.1-t2i-plus":  true,
		"wanx2.1-t2i-turbo": true,
	}
)

type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type DashScopeRequest struct {
	Model  string     `json:"model"`
	Input  Input      `json:"input"`
	Params Parameters `json:"parameters"`
}

type Input struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
}

type Parameters struct {
	Size string `json:"size,omitempty"`
	N    int    `json:"n,omitempty"`
}

type TaskResponse struct {
	Output struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
}

type TaskStatus struct {
	Output struct {
		TaskStatus string `json:"task_status"`
		Results    []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"output"`
}

func main() {
	flag.Parse()
	log.Printf("== 通义万象代理服务启动 ==")
	log.Printf("监听端口: %s", *port)
	log.Printf("支持的模型: %v", allowedModels)
	http.HandleFunc("/v1/chat/completions", handleChatCompletion)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

func handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] 收到请求 | 路径: %s | 客户端: %s", time.Now().Format(time.RFC3339), r.URL.Path, r.RemoteAddr)
	// Authentication
	authHeader := r.Header.Get("Authorization")
	if authHeader != "Bearer "+*openaiKey {
		http.Error(w, `{"error": "Invalid API key"}`, http.StatusUnauthorized)
		return
	}

	// Parse request
	var req OpenAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate model
	if !allowedModels[req.Model] {
		log.Printf("[%s] 不支持的模型 | 请求模型: %s", time.Now().Format(time.RFC3339), req.Model)
		http.Error(w, `{"error": "Unsupported model"}`, http.StatusBadRequest)
		return
	}
	log.Printf("[%s] 模型验证通过 | 使用模型: %s", time.Now().Format(time.RFC3339), req.Model)

	// Determine mode
	var (
		prompt, negativePrompt string
		size                   = "1024*1024"
		n                      = 1
	)

	if isAdvancedMode(req.Messages) {
		// Parse advanced parameters
		content := getUserContent(req.Messages)
		prompt = extractParam(content, "提示词")
		negativePrompt = extractParam(content, "反向提示词")
		size = extractParam(content, "图像分辨率")
		n = parseInt(extractParam(content, "图片数量"))
		log.Printf("[%s] 进入高级模式 | 参数解析完成", time.Now().Format(time.RFC3339))
	} else {
		// Simple mode
		prompt = getUserContent(req.Messages)
		log.Printf("[%s] 进入简单模式", time.Now().Format(time.RFC3339))
	}

	// Forward to DashScope
	taskID, err := submitDashScopeTask(req.Model, prompt, negativePrompt, size, n)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	log.Printf("[%s] 提交绘图任务 | 任务ID: %s", time.Now().Format(time.RFC3339), taskID)

	// Setup SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Poll task status
	ctx := r.Context()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			sendSSEEvent(w, `{"error": "Timeout"}`)
			return
		case <-ticker.C:
			status, err := getTaskStatus(taskID)
			if err != nil {
				sendSSEEvent(w, fmt.Sprintf(`{"error": "%v"}`, err))
				return
			}

			log.Printf("[%s] 轮询任务状态 | 任务ID: %s | 状态: %s",
				time.Now().Format(time.RFC3339),
				taskID,
				status.Output.TaskStatus)

			switch status.Output.TaskStatus {
			case "SUCCEEDED":
				var urls []string
				for i, res := range status.Output.Results {
					urls = append(urls, fmt.Sprintf("[p%d](%s)", i+1, res.URL))
				}
				response := fmt.Sprintf(`{"choices":[{"message":{"content":"%s"}}]}`, strings.Join(urls, ","))
				log.Printf("[%s] 任务完成 | 生成图片: %d张", time.Now().Format(time.RFC3339), len(urls))
				sendSSEEvent(w, response)
				return
			case "FAILED":
				sendSSEEvent(w, `{"error": "Task failed"}`)
				return
			default:
				sendSSEEvent(w, `{"status": "绘图中..."}`)
			}
		}
	}
}

func sendSSEEvent(w http.ResponseWriter, data string) {
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func isAdvancedMode(messages []Message) bool {
	for _, msg := range messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "通义万象高级模式") {
			return true
		}
	}
	return false
}

func getUserContent(messages []Message) string {
	for _, msg := range messages {
		if msg.Role == "user" {
			return msg.Content
		}
	}
	return ""
}

func extractParam(content, key string) string {
	re := regexp.MustCompile(fmt.Sprintf(`\[%s=([^\]]+)`, key))
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func submitDashScopeTask(model, prompt, negativePrompt, size string, n int) (string, error) {
	reqBody := DashScopeRequest{
		Model: model,
		Input: Input{
			Prompt:         prompt,
			NegativePrompt: negativePrompt,
		},
		Params: Parameters{
			Size: size,
			N:    n,
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://dashscope.aliyuncs.com/api/v1/services/aigc/text2image/image-synthesis", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+*dashscopeKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var taskResp TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return "", err
	}
	return taskResp.Output.TaskID, nil
}

func getTaskStatus(taskID string) (*TaskStatus, error) {
	url := fmt.Sprintf("https://dashscope.aliyuncs.com/api/v1/tasks/%s", taskID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+*dashscopeKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var status TaskStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
