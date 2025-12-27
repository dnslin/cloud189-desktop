package store

// SessionStore 抽象会话存储，由业务方约定具体 Session 结构体。
type SessionStore[T any] interface {
	SaveSession(session T) error
	LoadSession() (T, error)
	ClearSession() error
}

// TokenStore 抽象 token/refresh token/cookie 的持久化。
type TokenStore[T any] interface {
	SaveTokens(tokens T) error
	LoadTokens() (T, error)
	ClearTokens() error
}

// ConfigStore 抽象用户偏好或客户端配置的存储。
type ConfigStore[T any] interface {
	SaveConfig(cfg T) error
	LoadConfig() (T, error)
	ClearConfig() error
}

// UploadState 上传断点续传状态。
type UploadState struct {
	LocalPath    string   // 本地文件路径（唯一标识）
	ParentID     string   // 云端父目录 ID
	FileName     string   // 文件名
	FileSize     int64    // 文件大小
	FileMD5      string   // 文件 MD5（用于校验文件是否修改）
	UploadFileID string   // 天翼云上传会话 ID
	UploadedSize int64    // 已上传字节数
	PartHashes   []string // 已上传分片的 MD5 列表（用于计算 SliceMD5）
	CreatedAt    int64    // 创建时间戳
}

// UploadStateStore 上传状态持久化接口。
type UploadStateStore interface {
	// SaveState 保存上传状态，key 为本地文件路径。
	SaveState(localPath string, state *UploadState) error
	// LoadState 加载上传状态。
	LoadState(localPath string) (*UploadState, error)
	// DeleteState 删除上传状态。
	DeleteState(localPath string) error
}
