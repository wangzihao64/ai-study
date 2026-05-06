package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Message struct {
	Role    string
	Content string
}

func TokenEstimate(msg Message) int {
	return len([]rune(msg.Content))*2/3 + 10
}

// 滑动窗口
type SlidingWindowMemory struct {
	messages []Message
	maxRound int //保留最近N轮
}

func NewSlidingWindowMemory(maxRound int) *SlidingWindowMemory {
	return &SlidingWindowMemory{maxRound: maxRound}
}
func (m *SlidingWindowMemory) Add(msg Message) {
	m.messages = append(m.messages, msg)
}
func (m *SlidingWindowMemory) GetHistory() []Message {
	maxMessages := m.maxRound * 2
	if len(m.messages) <= maxMessages {
		return m.messages
	}
	fmt.Println("滑动窗口开始裁剪")
	// 只保留最近 maxMessages 条
	trim := m.messages[len(m.messages)-maxMessages:]
	fmt.Printf("原本文本数量%d,裁剪以后%d,保留最近%d轮\n", len(m.messages), len(trim), m.maxRound)
	return trim
}

// 摘要压缩
type SummaryMemory struct {
	messages       []Message
	summary        string //早期对话的摘要
	maxRecent      int    //保留最近N条原始消息
	summarizeAfter int    //超过多少条消息就要触发摘要
	client         *openai.Client
}

func NewSummaryMemory(maxRecent, summarizeAfter int, client *openai.Client) *SummaryMemory {
	return &SummaryMemory{maxRecent: maxRecent, summarizeAfter: summarizeAfter, client: client}
}
func (m *SummaryMemory) Add(msg Message) {
	m.messages = append(m.messages, msg)
}
func (m *SummaryMemory) generateSumary(msg []Message) string {
	//构建对话文本
	var dialog strings.Builder
	for _, msg := range m.messages {
		dialog.WriteString(fmt.Sprintf("[%s]:%s", msg.Role, msg.Content))
	}
	ctx := context.Background()
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("请用2-3句话简洁地总结以下对话的关键信息，保留重要的事实、决策和用户偏好，省略闲聊内容。"),
			openai.UserMessage(dialog.String()),
		},
		Model:       "qwen-plus",
		Temperature: openai.Float(0.3),
	}
	completion, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		fmt.Printf("生成摘要失败:%v\n", err)
		return "生成摘要失败"
	}
	return completion.Choices[0].Message.Content
}
func (m *SummaryMemory) GetHistory() []Message {
	if len(m.messages) <= m.summarizeAfter {
		return m.messages
	}
	ToSummarize := m.messages[:len(m.messages)-m.maxRecent]
	recent := m.messages[len(m.messages)-m.maxRecent:]
	summary := m.generateSumary(ToSummarize)
	oldTokens := 0
	for _, msg := range m.messages {
		oldTokens += TokenEstimate(msg)
	}
	fmt.Printf("[摘要压缩] %d条早期消息 (约%d Token) -> 摘要 (%d)Token", len(ToSummarize), oldTokens, TokenEstimate(Message{Content: summary}))

	//返回最近摘要+原始消息
	result := []Message{
		{Role: "system", Content: "以下是之前对话的摘要：\n" + summary},
	}
	result = append(result, recent...)
	return result
}

// 基于重要性的选择
type ScoredMessage struct {
	Mesage     Message
	Importance float64 //0.0~1.0
}
type ImportanceMemory struct {
	messages []ScoredMessage
	maxToken int
}

func NewImportanceMemory(maxToken int) *ImportanceMemory {
	return &ImportanceMemory{maxToken: maxToken}
}
func (m *ImportanceMemory) Add(msg Message, importance float64) {
	m.messages = append(m.messages, ScoredMessage{Mesage: msg, Importance: importance})
}
func trucate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
func (m *ImportanceMemory) GetHistory() []Message {
	totalTokens := 0
	for _, msg := range m.messages {
		totalTokens += TokenEstimate(msg.Mesage)
	}
	if totalTokens <= m.maxToken {
		result := make([]Message, len(m.messages))
		for i, msg := range m.messages {
			result[i] = msg.Mesage
		}
		return result
	}
	//token一旦超限，按重要性，从低到高排序，逐个移除低重要消息
	//但保持原始顺序--先标记要保留的，再按原顺序输出
	keep := make([]bool, len(m.messages))
	for i := range keep {
		keep[i] = true
	}
	currentTokens := totalTokens
	for currentTokens > m.maxToken {
		//找到重要性最低的消息
		minIdx := -1
		minScore := 2.0
		for i, msg := range m.messages {
			if keep[i] && msg.Importance < minScore {
				minScore = msg.Importance
				minIdx = i
			}
		}
		if minIdx == -1 {
			break
		}
		keep[minIdx] = false
		currentTokens -= TokenEstimate(m.messages[minIdx].Mesage)
		fmt.Printf("[重要性筛选] 移除 (%.1f)分 : %s\n",
			m.messages[minIdx].Importance, m.messages[minIdx].Mesage.Content)
	}
	var result []Message
	for i, msg := range m.messages {
		if keep[i] {
			result = append(result, msg.Mesage)
		}
	}
	return result
}
func main() {
	client := openai.NewClient(option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"))
	dialog := []Message{
		{Role: "user", Content: "您好，我正在开发一个电商系统"},
		{Role: "assistant", Content: "你好！电商系统是一个很好的项目，我可以帮你。"},
		{Role: "user", Content: "我们用Go 1.22开发，数据库用PostgreSQL"},
		{Role: "assistant", Content: "了解，Go 1.22 + PostgreSQL是很好的技术选型。"},
		{Role: "user", Content: "今天天气真不错"},
		{Role: "assistant", Content: "是呢，希望好天气能带来好心情。"},
		{Role: "user", Content: "帮我设计一下订单模块的数据库表结构"},
		{Role: "assistant", Content: "好的，订单模块通常需要orders、order_items、payments等核心表..."},
		{Role: "user", Content: "记住，所有金额字段必须用decimal类型，不能用float"},
		{Role: "assistant", Content: "明白，金额字段统一使用DECIMAL(10,2)，避免浮点精度问题。"},
		{Role: "user", Content: "现在帮我写一下订单创建的API接口"},
	}
	fmt.Println("==策略1==滑动窗口，保留最近3轮\n")
	m := NewSlidingWindowMemory(3)
	for _, msg := range dialog {
		m.Add(msg)
	}
	msgs := m.GetHistory()
	for _, msg := range msgs {
		fmt.Println(msg)
	}

	fmt.Printf("==策略2==摘要压缩\n")
	s := NewSummaryMemory(4, 6, &client)
	for _, msg := range dialog {
		s.Add(msg)
	}
	msg1 := s.GetHistory()
	for _, msg := range msg1 {
		fmt.Println(msg)
	}

	fmt.Printf("==策略3==重要性选择\n")
	importance := NewImportanceMemory(100)
	arrary := []float64{0.1, 0.2, 0.3, 0.9, 0.7, 0.6, 0.65, 0.4, 0.2, 0.1, 0.88}
	for i, msg := range dialog {
		importance.Add(msg, arrary[i])
	}
	importance.GetHistory()
	//for _, msg := range msg2 {
	//	fmt.Println(msg)
	//}
}
