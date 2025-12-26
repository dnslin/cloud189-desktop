package store

// SessionStore 抽象会话存储，业务侧通过类型参数指定会话结构体并注入实现。
type SessionStore[T any] interface {
	SaveSession(session *T) error
	LoadSession() (*T, error)
	ClearSession() error
}

// TokenStore 抽象 token/refresh token/cookie 的持久化。
type TokenStore[T any] interface {
	SaveTokens(tokens *T) error
	LoadTokens() (*T, error)
	ClearTokens() error
}

// ConfigStore 抽象用户偏好或客户端配置的存储。
type ConfigStore[T any] interface {
	SaveConfig(cfg *T) error
	LoadConfig() (*T, error)
	ClearConfig() error
}
