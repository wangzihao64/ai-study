package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const qwenEmbeddingModel = "text-embedding-v4"

// MemoryRecord中的一条记录
type MemoryRecord struct {
	ID        string
	Content   string    //原始文本
	Embedding []float64 //向量表示
	Metadata  map[string]string
	Time      time.Time
}
type VectorMemoryStore struct {
	records []MemoryRecord
	client  *openai.Client
}

func NewVectorMemoryStore(client *openai.Client) *VectorMemoryStore {
	return &VectorMemoryStore{client: client}
}

func (s *VectorMemoryStore) Store(ctx context.Context, id string, content string, metadata map[string]string) error {
	embedding, err := s.GetEmbedding(ctx, content)
	if err != nil {
		return err
	}

	record := MemoryRecord{
		ID:        id,
		Content:   content,
		Embedding: embedding,
		Metadata:  metadata,
		Time:      time.Now(),
	}
	s.records = append(s.records, record)
	fmt.Printf("  📝 存入记忆 [%s]: %s\n", id, truncate(content, 50))
	return nil
}
func (s *VectorMemoryStore) Retrieve(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	queryEmbedding, err := s.GetEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve embeddings: %w", err)
	}
	//计算与所有记忆的余弦相似度
	type scored struct {
		record MemoryRecord
		score  float64
	}
	var results []scored
	for _, record := range s.records {
		sim := cosineSimilarity(queryEmbedding, record.Embedding)
		results = append(results, scored{record: record, score: sim})
	}
	//按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	//取TopK
	var TopResults []MemoryRecord
	limit := topK
	if limit > len(results) {
		limit = len(results)
	}
	for i := 0; i < limit; i++ {
		TopResults = append(TopResults, results[i].record)
		fmt.Printf("检索到 [%s],相似度 %.4f : %s\n", results[i].record.ID, results[i].score, truncate(results[i].record.Content, 50))
	}
	return TopResults, nil
}
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
func cosineSimilarity(queryEmbedding, recordEmbedding []float64) float64 {
	if len(queryEmbedding) != len(recordEmbedding) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range queryEmbedding {
		dotProduct += float64(queryEmbedding[i]) * float64(recordEmbedding[i])
		normA += float64(queryEmbedding[i]) * float64(queryEmbedding[i])
		normB += float64(recordEmbedding[i]) * float64(recordEmbedding[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / math.Sqrt(normA) * math.Sqrt(normB)
}
func (s *VectorMemoryStore) GetEmbedding(ctx context.Context, content string) ([]float64, error) {
	if s.client == nil {
		return nil, errors.New("openai client is nil")
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("content is empty")
	}

	resp, err := s.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(content),
		},
		Model:          qwenEmbeddingModel,
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("embedding response contains no data")
	}

	return resp.Data[0].Embedding, nil
}

func main() {
	client := openai.NewClient(option.WithAPIKey(os.Getenv("DASHSCOPE_API_KEY")),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"))
	s := NewVectorMemoryStore(&client)
	ctx := context.Background()
	fmt.Println("==存储阶段==")
	memories := []struct {
		id       string
		content  string
		metadata map[string]string
	}{
		{"m1", "用户是一名Go语言开发者，有5年后端开发经验",
			map[string]string{"type": "user_profile"}},
		{"m2", "项目使用Go 1.22 + PostgreSQL + Redis技术栈",
			map[string]string{"type": "project_info"}},
		{"m3", "用户要求所有金额字段必须使用decimal类型，不能用float",
			map[string]string{"type": "requirement"}},
		{"m4", "订单模块已经设计完成，包含orders、order_items、payments三张核心表",
			map[string]string{"type": "progress"}},
		{"m5", "用户偏好简洁的代码风格，不喜欢过度封装",
			map[string]string{"type": "preference"}},
	}
	for _, m := range memories {
		if err := s.Store(ctx, m.id, m.content, m.metadata); err != nil {
			log.Fatal("存储失败：%v", err)
		}
	}
	//检索测试
	fmt.Println("==检索阶段==")
	queries := []string{
		"用户的技术背景是什么",
		"数据库表结构怎么设计的",
		"金额相关的开发规范",
	}
	for _, q := range queries {
		fmt.Printf("\n查询: %s\n", q)
		results, err := s.Retrieve(ctx, q, 2)
		if err != nil {
			log.Fatal("检索失败:%v", err)
			continue
		}
		_ = results
	}
}
