package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	)
	fmt.Println(os.Getenv("DASHSCOPE_API_KEY"))
	ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		stream := client.Chat.Completions.NewStreaming(
			ctx, openai.ChatCompletionNewParams{
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("你是谁"),
				},
				Model:         "deepseek-v3.2",
				StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
			})
		for stream.Next() {
			evt := stream.Current()
			if len(evt.Choices) > 0 {
				print(evt.Choices[0].Delta.Content)
			}
		}
		println()
	}()
	wg.Wait()
}
