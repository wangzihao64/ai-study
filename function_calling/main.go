package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	)
	ctx := context.Background()
	question := "今天南京的天气如何？"
	print(">")
	println(question)
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(question),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        "get_weather",
				Description: openai.String("根据城市获取天气"),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]string{
							"type": "string",
						},
					},
					"required": []string{"location"},
				},
			}),
		},
		Seed:  openai.Int(0),
		Model: "deepseek-v3.2",
	}
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		panic(err)
	}
	fmt.Println("DEBUG 1: API 返回成功")
	fmt.Printf("DEBUG 1.1: %+v\n", completion)
	toolCalls := completion.Choices[0].Message.ToolCalls

	if len(toolCalls) == 0 {
		fmt.Println("No function call")
		return
	}

	params.Messages = append(params.Messages, completion.Choices[0].Message.ToParam())

	for _, toolCall := range toolCalls {
		if toolCall.Function.Name == "get_weather" {
			var args map[string]interface{}
			err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
			if err != nil {
				panic(err)
			}
			location := args["location"].(string)
			weatherData := getWeather(location)
			fmt.Print("Weather in %s : %s\n", location, weatherData)
			params.Messages = append(params.Messages, openai.ToolMessage(weatherData, toolCall.ID))
		}
	}

	completion, err = client.Chat.Completions.New(ctx, params)
	if err != nil {
		panic(err)
	}
	println(completion.Choices[0].Message.Content)
}

func getWeather(location string) string {
	return "Sunny, 25°C"
}
