package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SessionState Session的状态数据
type SessionState struct {
	AppState     map[string]interface{} //应用级别：所有用户共享
	UserState    map[string]interface{} //用户级别：同一用户跨会话共享
	SessionState map[string]interface{} //会话级别：当前会话独有
}
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	TimeStamp time.Time `json:"timestamp"`
}

// Session完整的会话对象
type Session struct {
	ID       string        `json:"id"`
	UserID   string        `json:"user_id"`
	AppID    string        `json:"app_id"`
	State    SessionState  `json:"state"`
	History  []ChatMessage `json:"history"`
	CreateAt time.Time     `json:"create_at"`
	UpdateAt time.Time     `json:"update_at"`
}

func NewSession(id, userID, appID string) *Session {
	now := time.Now()
	return &Session{
		ID:     id,
		UserID: userID,
		AppID:  appID,
		State: SessionState{
			AppState:     make(map[string]interface{}),
			UserState:    make(map[string]interface{}),
			SessionState: make(map[string]interface{}),
		},
		CreateAt: now,
		UpdateAt: now,
	}
}

// AddMessage添加对话消息
func (session *Session) AddMessage(role, content string) {
	session.History = append(session.History, ChatMessage{
		Role:      role,
		Content:   content,
		TimeStamp: time.Now(),
	})
	session.UpdateAt = time.Now()
}

// SetState设置指定作用域的状态
func (session *Session) SetState(scope, key string, value interface{}) {
	switch scope {
	case "app":
		session.State.AppState[key] = value
	case "user":
		session.State.UserState[key] = value
	case "session":
		session.State.SessionState[key] = value
	}
	session.UpdateAt = time.Now()
}

// GetState获取指定工作域的状态，支持向上查找
func (session *Session) GetState(key string) (interface{}, string) {
	//先查session级别
	if value, ok := session.State.SessionState[key]; ok {
		return value, "session"
	}
	//再查user级别
	if value, ok := session.State.UserState[key]; ok {
		return value, "user"
	}
	//最后查app级别
	if value, ok := session.State.AppState[key]; ok {
		return value, "app"
	}
	return nil, ""
}

// SessionService Session管理服务
type SessionService struct {
	mu       sync.RWMutex
	sessions map[string]*Session               //sessionID->Session
	userIdx  map[string][]string               //userID->[]sessionID
	appState map[string]map[string]interface{} //appID->appState
}

func NewSessionService() *SessionService {
	return &SessionService{
		sessions: make(map[string]*Session),
		userIdx:  make(map[string][]string),
		appState: make(map[string]map[string]interface{}),
	}
}

// CreateSession创建新会话
func (svc *SessionService) CreateSession(sessionID, userID, appID string) *Session {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	session := NewSession(sessionID, userID, appID)
	//继承app级别状态
	if appState, ok := svc.appState[appID]; ok {
		for k, v := range appState {
			session.State.AppState[k] = v
		}
	}
	//继承user级别状态（从该用户最近一次会话获取）
	if sessionIDs, ok := svc.userIdx[userID]; ok && len(sessionIDs) > 0 {
		lastSessionID := sessionIDs[len(sessionIDs)-1]
		if lastSession, ok := svc.sessions[lastSessionID]; ok {
			for k, v := range lastSession.State.UserState {
				session.State.UserState[k] = v
			}
		}
	}
	svc.sessions[sessionID] = session
	svc.userIdx[userID] = append(svc.userIdx[userID], sessionID)
	return session
}

// setAppState 设置应用级别状态
func (svc *SessionService) SetAppState(appID, key string, value interface{}) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, ok := svc.appState[appID]; !ok {
		svc.appState[appID] = make(map[string]interface{})
	}
	svc.appState[appID][key] = value
}

// GetSession获取会话
func (svc *SessionService) GetSession(sessionID string) (*Session, bool) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if value, ok := svc.sessions[sessionID]; ok {
		return value, ok
	}
	return nil, false
}
func main() {
	svc := NewSessionService()
	svc.SetAppState("ecommerce-bot", "model", "qian-plus")
	svc.SetAppState("ecommerce-bot", "max_tokens", "2048")
	svc.SetAppState("ecommerce-bot", "system_prompt", "你是一个电商客服助手")
	fmt.Println("=== 设置应用级别配置 ===")
	fmt.Println("  model=qwen-plus, max_tokens=2048")
	//用户张三的第一次会话
	fmt.Println("===用户张三的第一次会话===")
	session1 := svc.CreateSession("s001", "zhangsan", "ecommerce-bot")
	session1.AddMessage("user", "你好，我想买一台笔记本电脑")
	session1.AddMessage("assistant", "你好！请问你的预算范围和主要用途是什么？")
	session1.AddMessage("user", "预算8000左右，主要写代码用")
	session1.SetState("user", "budget_range", "8000元左右")
	session1.SetState("user", "use_case", "编程开发")
	session1.SetState("session", "current_intent", "选购笔记本")
	//查看状态
	val, scope := session1.GetState("model")
	fmt.Printf("model=%v ,来自%s级别\n", val, scope)
	val, scope = session1.GetState("budget_range")
	fmt.Printf("budget_range=%v ,来自%s级别\n", val, scope)
	val, scope = session1.GetState("current_intent")
	fmt.Printf("current_intent=%v ,来自%s级别\n", val, scope)

	//张三的第二次会话
	fmt.Println("===用户张三的第二次会话===新会话")
	session2 := svc.CreateSession("s002", "zhangsan", "ecommerce-bot")
	session2.AddMessage("user", "上次推荐的笔记本我想下单")
	val, scope = session2.GetState("budget_range")
	fmt.Printf("budget_rage=%v,来自%s级别 <--跨会话继承\n", val, scope)
	val, scope = session2.GetState("current_intent")
	fmt.Printf("current_intent=%v,来自%s级别 <--不跨会话继承，因为是session级别\n", val, scope)
	session2.SetState("user", "name", "zhangsan")

	session3 := svc.CreateSession("s003", "zhangsan", "ecommerce-bot")
	val, scope = session3.GetState("budget_range")
	fmt.Printf("budget_rage=%v,来自%s级别 <--跨会话继承\n", val, scope)
	// 用户李四的会话（不同用户，共享app级别状态）
	fmt.Println("\n=== 李四的会话 ===")
	session4 := svc.CreateSession("s003", "lisi", "ecommerce-bot")
	session4.AddMessage("user", "有没有好的降噪耳机推荐？")

	val, scope = session4.GetState("model")
	fmt.Printf("  查询 'model': %v (来自%s级别) ← 共享应用配置\n", val, scope)
	val, scope = session4.GetState("budget_range")
	fmt.Printf("  查询 'budget_range': %v (scope=%s) ← 张三的数据，李四看不到\n", val, scope)

	// 序列化展示
	fmt.Println("\n=== Session状态快照（JSON）===")
	snapshot, _ := json.MarshalIndent(session2.State, " ", " ")
	fmt.Println(string(snapshot))
	fmt.Printf("  %s\n", string(snapshot))
}
