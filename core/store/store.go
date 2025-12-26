package store

// SessionStore 抽象会话存储，由业务方约定具体 Session 结构体。
type SessionStore interface {
	SaveSession(session any) error
	LoadSession() (any, error)
	ClearSession() error
}

// TokenStore 抽象 token/refresh token/cookie 的持久化，由业务方约定结构体。
type TokenStore interface {
	SaveTokens(tokens any) error
	LoadTokens() (any, error)
	ClearTokens() error
}

// ConfigStore 抽象用户偏好或客户端配置的存储，由业务方约定结构体。
type ConfigStore interface {
	SaveConfig(cfg any) error
	LoadConfig() (any, error)
	ClearConfig() error
}
