package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type TaskStep struct {
	StepNumber  int    `json:"step_number"`
	Description string `json:"description"`
	ToolNeeded  string `json:"tool_needed"`
	DependsOn   []int  `json:"depends_on"`
	Output      string `json:"expected_output"`
}
type TaskPlan struct {
	Goal  string     `json:"goal"`
	Steps []TaskStep `json:"steps"`
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

func decomposeTask(task string) (*TaskPlan, error) {
	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	)
	ctx := context.Background()
	systemPrompt := `你是一个任务规划专家。用户会给你一个复杂任务，你需要将其分解为具体的执行步骤。
请以JSON格式返回，结构如下：
{
  "goal": "任务目标",
  "steps": [
    {
      "step_number": 1,
      "description": "步骤描述",
      "tool_needed": "需要的工具（如：web_search, database_query, text_generation, code_execution, none）",
      "depends_on": [],
      "expected_output": "这一步的预期输出"
    }
  ]
}
注意：depends_on 字段表示该步骤依赖哪些前置步骤的编号。没有依赖的步骤填空数组。`
	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage("请分解这个任务" + task),
		},
		Model: "deepseek-v3.2",
	}
	completion, err := client.Chat.Completions.New(ctx, param)
	if err != nil {
		panic(err)
	}
	var plan TaskPlan
	rawcontent := completion.Choices[0].Message.Content
	content := cleanJSONContent(rawcontent)
	fmt.Println(content)
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("JSON unmarshal error: %v", err))
	}
	return &plan, nil
}
func main() {
	task := "帮我查询一下南京的天气如何？"
	plan, err := decomposeTask(task)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("任务目标：%s\n\n", plan.Goal)
	for _, step := range plan.Steps {
		deps := "无"
		if len(step.DependsOn) > 0 {
			depsJSON, _ := json.Marshal(step.DependsOn)
			deps = string(depsJSON)
		}
		fmt.Printf("步骤%d：%s\n", step.StepNumber, step.Description)
		fmt.Printf("  工具：%s\n", step.ToolNeeded)
		fmt.Printf("  依赖：%s\n", deps)
		fmt.Printf("  预期输出：%s\n\n", step.Output)
	}
}
