package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type TaskPlan struct {
	client openai.Client
}
type Executor struct {
	client openai.Client
}
type Planner struct {
	client openai.Client
}
type TaskStep struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Result      string `json:"result"`
}

func NewExecutor(client openai.Client) *Executor {
	return &Executor{client: client}
}
func NewPlanner(client openai.Client) *Planner {
	return &Planner{client: client}
}

type planResponse struct {
	Steps []TaskStep `json:"steps"`
}

func NewTaskPlan(client openai.Client) *TaskPlan {
	return &TaskPlan{client: client}
}
func cleanJSONContent(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSpace(s)
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}
func (p *TaskPlan) plan(ctx context.Context, goal string) (*planResponse, error) {
	systemPropmt := `你是一个任务规划专家。根据用户的目标，生成3-5个简洁具体的执行步骤。
返回JSON格式：{"steps": [{"id": 1, "description": "步骤描述", "status": "pending", "result": ""}]}`
	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPropmt),
			openai.UserMessage(goal),
		},
		Model: "deepseek-v3.2",
	}
	completion, err := p.client.Chat.Completions.New(ctx, param)
	if err != nil {
		panic(err)
	}
	rawcontent := completion.Choices[0].Message.Content
	content := cleanJSONContent(rawcontent)
	var resp planResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("JSON unmarshal error: %v", err)
	}
	return &resp, nil
}

func (e *Executor) execute(ctx context.Context, step *TaskStep, Prevresults []string) (string, error) {
	contentInfo := ""
	if len(Prevresults) != 0 {
		contentInfo = "\n前序执行结果:\n" + strings.Join(Prevresults, "\n")
	}
	systemPropmt := "你是一个任务执行助手。请认真完成给定的步骤，给出简洁的执行结果。"
	userPrompt := fmt.Sprintf("请执行以下步骤:%s%s", step.Description, contentInfo)
	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPropmt),
			openai.UserMessage(userPrompt),
		},
		Model: "deepseek-v3.2",
	}
	completion, err := e.client.Chat.Completions.New(ctx, param)
	if err != nil {
		return "", err
	}
	return completion.Choices[0].Message.Content, nil
}
func (p *Planner) Replan(ctx context.Context, goal string, plan *planResponse) (*planResponse, error) {
	//构建当前进展摘要
	var progress strings.Builder
	for _, step := range plan.Steps {
		progress.WriteString(fmt.Sprintf("步骤%d %s %s\n", step.ID, step.Status, step.Description))
		if step.Result != "" {
			progress.WriteString(fmt.Sprintf("当前结果-> %s", step.Result))
		}
		progress.WriteString("\n")
	}
	systemPrompt := "`你是一个任务规划专家。" +
		"根据原始目标和当前执行进展，判断是否需要调整后续计划。\n" +
		"如果当前计划仍然合理，原样返回剩余的pending步骤。\n" +
		"如果需要调整，返回新的步骤列表（保留已完成的步骤，调整pending的步骤）。\n" +
		"返回JSON格式：{\"steps\": [{\"id\": 1, \"description\": \"步骤描述\", \"status\": \"状态\", \"result\": \"结果\"}]}`"
	userPrompt := fmt.Sprintf("原始目标 %s,当前进展 %s,请评估并返回更新后的计划。", goal, progress.String())
	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
	}
	completion, err := p.client.Chat.Completions.New(ctx, param)
	if err != nil {
		return nil, err
	}
	rawcontent := completion.Choices[0].Message.Content
	content := cleanJSONContent(rawcontent)
	var NewPlan planResponse
	if err := json.Unmarshal([]byte(content), &NewPlan); err != nil {
		return nil, fmt.Errorf("JSON unmarshal error: %v", err)
	}
	return &NewPlan, nil
}
func plan_and_execute(goal string) {
	ctx := context.Background()
	taskPlan := NewTaskPlan(openai.NewClient(
		option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	))
	fmt.Println("规划阶段")
	resp, err := taskPlan.plan(ctx, goal)
	if err != nil {
		panic(err)
	}
	executePlan := NewExecutor(openai.NewClient(
		option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	))
	for _, step := range resp.Steps {
		fmt.Printf("  步骤%d：%s\n", step.ID, step.Description)
	}
	fmt.Println("执行阶段")
	var results []string
	for i := range resp.Steps {
		step := &resp.Steps[i]
		fmt.Printf("\n 执行步骤: %d:%s\n", step.ID, step.Description)
		result, err := executePlan.execute(ctx, step, results)
		if err != nil {
			step.Status = "failed"
			step.Result = err.Error()
			fmt.Println("失败", err.Error())
			fmt.Println("出发重新规划")
			newPlanClient := NewPlanner(openai.NewClient(
				option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
				option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
			))
			newPlan, err := newPlanClient.Replan(ctx, goal, resp)
			if err == nil {
				resp = newPlan
				fmt.Println("计划已调整，请重新执行")
			}
			continue
		}
		results = append(results, fmt.Sprintf("步骤%d结果：%s", step.ID, result))
		step.Result = result
		step.Status = "success"
		fmt.Println("完成 ", step.Result)
	}
	fmt.Println("任务执行完成")

}
func main() {
	goal := "用go语言写一个简单的http服务器，支持JSON相应和日志中间件"
	plan_and_execute(goal)
}
