package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type ReflectionAgent struct {
	client   *openai.Client
	maxRound int
}

func NewReflectionAgent(maxRound int) *ReflectionAgent {
	client := openai.NewClient(option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"))
	return &ReflectionAgent{client: &client, maxRound: maxRound}
}
func (r *ReflectionAgent) Generate(ctx context.Context, task string, feedback string) (string, error) {
	userContent := "请完成以下任务" + task
	if feedback != "" {
		userContent += "\n 上一轮的反馈意见如下，请根据此改进你的方案:\n" + feedback
	}
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(`你是一个Go语言专家。请认真完成用户的编程任务，给出高质量的代码和设计方案。
如果收到反馈，请仔细理解每条意见并在新方案中逐一改进。`),
			openai.UserMessage(userContent),
		},
		Model:       "qwen-plus",
		Temperature: openai.Float(0.7),
	}
	completion, err := r.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}
	return completion.Choices[0].Message.Content, nil
}

// aa
// abcaa
func searching(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
func contains(s, sub string) bool {
	return len(s) >= len(sub) && searching(s, sub)
}
func containsApproval(s string) bool {
	approvalKeyWords := []string{"LTGM", "没有明显问题", "方案足够好", "质量很高", "无需修改"}
	for _, k := range approvalKeyWords {
		if contains(s, k) {
			return true
		}
	}
	return false
}
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
func (r *ReflectionAgent) Critique(ctx context.Context, task string, solution string) (string, bool, error) {
	userPrompt := fmt.Sprintf("任务：%s\n，待审查的方案:%s\n", task, solution)
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(`你是一个严格的代码审查专家。你的任务是审查给定方案的质量，找出问题和不足。
审查维度：代码正确性、性能、错误处理、可读性、Go最佳实践。

如果方案已经足够好，没有明显问题需要修改，请回复"LGTM"（Looks Good To Me）。
如果有改进空间，请列出具体的问题和改进建议，不要泛泛而谈。`),
			openai.UserMessage(userPrompt),
		},
		Model:       "qwen-plus",
		Temperature: openai.Float(0.3),
	}
	completion, err := r.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", false, err
	}
	feedback := completion.Choices[0].Message.Content
	approved := len(feedback) < 50 || containsApproval(feedback)
	return feedback, approved, nil
}
func (r *ReflectionAgent) Run(task string) string {
	ctx := context.Background()
	var feedback string
	var solution string
	for i := 0; i < r.maxRound; i++ {
		fmt.Printf("\n===第%d轮===\n", i+1)
		solution, err := r.Generate(ctx, task, feedback)
		// Generator 生成/改进方案
		fmt.Println("📝 Generator 正在生成方案...")
		if err != nil {
			log.Printf("生成失败: %v", err)
			continue
		}
		fmt.Println("方案长度 ", len(solution))
		// Critic 审查方案
		fmt.Println("🔍 Critic 正在审查...")
		var approved bool
		feedback, approved, err := r.Critique(ctx, task, solution)
		if err != nil {
			log.Printf("审查失败: %v", err)
			continue
		}
		if approved {
			fmt.Printf("Critic: LGTM! 方案通过审查")
			return solution
		}
		fmt.Printf("💬 Critic 反馈：%s\n", truncateString(feedback, 200))
	}
	fmt.Println("达到最大轮次，返回最后一版方案")
	return solution
}
func main() {
	task := "用Go实现一个线程安全的LRU缓存，支持Get、Put操作和过期时间"
	r := NewReflectionAgent(3)
	solution := r.Run(task)
	fmt.Println("最终方案： ", solution)
}
