package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

const GLM_API_KEY = "fde4d98862b64ca995f0286b916ab176.L7lfCCAmMv3VqNwe" // 去 https://open.bigmodel.cn 获取
const token = "mai2696047693"

// 全局变量，启动时读取一次
var systemPrompt string

// NapCatQQ 推过来的消息结构
type Event struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	UserId      int64  `json:"user_id"`
	GroupId     int64  `json:"group_id"`
	Message     string `json:"raw_message"`
	MessageId   int64  `json:"message_id"`
}

// 要发给 NapCatQQ 的动作
type Action struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params"`
	Echo   string         `json:"echo"`
}

func readSystemPrompt() string {
	// 读取 prompt.md
	data, err := os.ReadFile("./sm_master_system_prompt.md")
	if err != nil {
		log.Fatal("读取 prompt.md 失败:", err)
	}
	systemPrompt := string(data) // []byte 转 string
	log.Println("人设加载成功")
	return systemPrompt

}

func main() {
	systemPrompt = readSystemPrompt()

	// 构造请求头
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	// 连接 NapCatQQ
	// 这是在调用一个函数，该函数返回3个值，用 := 同时接收
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws://127.0.0.1:3001",
		header,
	)

	if err != nil {
		log.Fatal("连接失败:", err)
	}
	// defer = 延迟执行，等当前函数结束时再执行。
	defer conn.Close()

	log.Println("已连接 NapCatQQ")

	// 循环读取消息
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取失败:", err)
			break
		}

		var event Event
		json.Unmarshal(raw, &event)

		// 只处理私聊消息
		if event.PostType == "message" && event.MessageType == "private" {
			// 开一个新的 goroutine
			go handleMessage(conn, event) // 每条消息开一个 goroutine
			/*
				conn* websocket.Conn用来回复消息（发送需要连接）
				eventEvent收到的消息内容（谁发的、发了什么）
			*/
		}
	}
}

func handleMessage(conn *websocket.Conn, event Event) {
	// 调用 LLM 拿回复
	reply := callLLM(event.Message)

	// 构造发消息的 action
	action := Action{
		Action: "send_private_msg",
		Params: map[string]any{
			"user_id": event.UserId,
			"message": reply,
		},
	}

	// 将 action 对象序列化为 JSON 格式的字节数组（[]byte）
	data, _ := json.Marshal(action)
	conn.WriteMessage(websocket.TextMessage, data)
}

//func callLLM(input string) string {
//	// 这里接 DeepSeek / Claude API
//	// 返回回复文本
//
//	return "（LLM回复）"
//}

// -------- GLM 请求/响应结构体 --------

// GLMRequest 发给 GLM 的请求
type GLMRequest struct {
	Model    string       `json:"model"`
	Messages []GLMMessage `json:"messages"`
}

// GLMMessage — 单条对话消息
type GLMMessage struct {
	Role    string `json:"role"` // 这条消息是谁说的
	Content string `json:"content"`
}

// GLMResponse — GLM 返回的响应
type GLMResponse struct {
	Choices []struct {
		Message GLMMessage `json:"message"`
	} `json:"choices"`
}

// glm-4.5-air
func callLLM(input string) string {

	// 1. 构造请求体
	reqBody := GLMRequest{
		Model: "glm-4.5-air", // 模型
		Messages: []GLMMessage{
			{Role: "system", Content: systemPrompt}, // ← 用文件内容
			{Role: "user", Content: input},
		},
	}

	// 2. 序列化成 JSON
	data, err := json.Marshal(reqBody)
	if err != nil {
		log.Println("序列化失败:", err)
		return "出错了"
	}

	// 3. 发 HTTP 请求
	req, err := http.NewRequest(
		"POST",
		"https://open.bigmodel.cn/api/paas/v4/chat/completions",
		bytes.NewBuffer(data),
	)
	if err != nil {
		log.Println("创建请求失败:", err)
		return "出错了"
	}

	// 4. 加请求头（身份验证）
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+GLM_API_KEY)

	// 5. 发送请求
	client := &http.Client{} // 创建 HTTP 客户端
	// defer = 延迟执行
	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求失败:", err)
		return "出错了"
	}
	// defer = 延迟执行
	defer resp.Body.Close()

	// 6. 读取响应
	// 把数据流一次性全部读完
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取响应失败:", err)
		return "出错了"
	}

	// 7. 解析响应
	var glmResp GLMResponse
	err = json.Unmarshal(body, &glmResp)
	if err != nil {
		log.Println("解析响应失败:", err)
		return "出错了"
	}

	// 8. 返回 GLM 的回复
	if len(glmResp.Choices) == 0 {
		return "没有回复"
	}
	return glmResp.Choices[0].Message.Content
}
