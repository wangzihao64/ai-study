package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type ThoughNode struct {
	ID       int
	Thought  string
	Score    float64
	Children []*ThoughNode
	Parent   *ThoughNode
	Depth    int
}
type ToTSolver struct {
	client    *openai.Client
	model     string
	maxDepth  int //最大思考深度
	branchNum int //每一步生成的候选思路数
	beamWidth int //每层保留的最优节点数
}

func NewToTSolver(maxDepth, branchNum, beamWidth int) *ToTSolver {
	client := openai.NewClient(option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"))

	return &ToTSolver{
		client:    &client,
		model:     "deepseek-v3.2",
		maxDepth:  maxDepth,
		branchNum: branchNum,
		beamWidth: beamWidth,
	}
}

func (t *ToTSolver) GenerateThoughts(ctx context.Context, problem string, currentPath string) ([]string, error) {
	userPrompt := fmt.Sprintf(`针对以下问题，基于当前的思考路径，请生成 %d 个不同的的下一步思路。
每一个思路应该是一个不同的思路或方法。
问题：%s
当前思考路径：%s
请以JSON数组格式返回%d个思路，每个思路都是一段简洁的文字:
["思路1", "思路2", "思路3"]`, t.branchNum, problem, currentPath, t.branchNum)
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(userPrompt),
		},
		Model:       t.model,
		Temperature: openai.Float(0.9), //温度高以获得多样性
	}
	completion, err := t.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("模型未返回候选内容")
	}
	var thoughts []string
	content := normalizeJSONResponse(completion.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &thoughts); err != nil {
		return nil, err
	}
	return thoughts, nil
}

func (t *ToTSolver) EvaluateThoughts(ctx context.Context, problem string, thought string) (float64, error) {
	prompt := fmt.Sprintf(`请评估针对以思路对于解决问题的质量
    问题: %s\n
    解决思路：%s\n
    请给出1-10的评分（10分最好），只返回一个JSON对象：{"score": 8, "reason": "评分理由"}`, problem, thought)
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       t.model,
		Temperature: openai.Float(0.3),
	}
	completion, err := t.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return 0, err
	}
	var result struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if len(completion.Choices) == 0 {
		return 5, nil
	}
	content := normalizeJSONResponse(completion.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		//解析失败给个中间分
		return 5, nil
	}
	return result.Score, nil
}

func normalizeJSONResponse(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func buildPath(node *ThoughNode) string {
	var path []string
	for n := node; n != nil; n = n.Parent {
		path = append([]string{n.Thought}, path...)
	}
	result := ""
	for i, p := range path {
		if i > 0 {
			result += "->"
		}
		result += p
	}
	return result
}
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
func (t *ToTSolver) Solve(problem string) *ThoughNode {
	ctx := context.Background()
	nodeId := 0
	root := &ThoughNode{ID: nodeId, Thought: "开始分析问题", Depth: 0}
	nodeId++
	currentLevel := []*ThoughNode{root}
	for depth := 0; depth < t.maxDepth; depth++ {
		fmt.Printf("\n===第%d层思考\n", depth+1)
		var nextLevel []*ThoughNode
		for _, node := range currentLevel {
			//构建从根节点到当前节点的思考路径
			path := buildPath(node)
			//生成候选思路
			thoughts, err := t.GenerateThoughts(ctx, problem, path)
			if err != nil {
				log.Printf("生成候选思路失败: %v\n", err)
				continue
			}
			for _, thought := range thoughts {
				child := &ThoughNode{
					ID:      nodeId,
					Thought: thought,
					Depth:   depth + 1,
					Parent:  node,
				}
				nodeId++
				//评估思路质量
				score, err := t.EvaluateThoughts(ctx, problem, thought)
				if err != nil {
					score = 5
				}
				child.Score = score
				nextLevel = append(nextLevel, child)
				fmt.Printf("Child思路[%d] (%1.f分)：%s\n", child.ID, score, truncate(thought, 60))
			}
		}
		sort.Slice(nextLevel, func(i, j int) bool {
			return nextLevel[i].Score > nextLevel[j].Score
		})
		if len(nextLevel) > t.beamWidth {
			fmt.Printf("保留前%d个最优思路\n", t.beamWidth)
			nextLevel = nextLevel[:t.beamWidth]
		}
		currentLevel = nextLevel
	}
	if len(currentLevel) > 0 {
		return currentLevel[0]
	}
	return root
}
func main() {
	t := NewToTSolver(2, 3, 2)
	problem := "今日南京天气"
	node := t.Solve(problem)
	result := buildPath(node)
	fmt.Println(result)
	fmt.Println("最终评分 ", node.Score)
}
