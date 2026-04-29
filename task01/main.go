package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type TaskStep struct {
	StepNumber  int    `json:"step_number"`
	Description string `json:"description"`
	ToolNeeded  string `json:"tool_needed"`
	DependsOn   []int  `json:"depends_on"`
	Output      string `json:"output"`
}
type TaskPlan struct {
	Goal  string     `json:"goal"`
	Steps []TaskStep `json:"steps"`
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
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("JSON unmarshal error: %v", err))
	}
	return &plan, nil
}
func main() {
	task := "帮我查询一下南京的天气如何？"
	taskplan, err := decomposeTask(task)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(taskplan.Goal)
	for _, step := range taskplan.Steps {
		deps := "无"
		if len(step.DependsOn) > 0 {
			depsJSON, _ := json.Marshal(step.DependsOn)
			deps = string(depsJSON)
		}
		fmt.Println(step.StepNumber)
		fmt.Println(step.Description)
		fmt.Println(step.ToolNeeded)
		fmt.Println(step.Output)
		fmt.Println(deps)
	}
}
