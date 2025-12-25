package cloud189

// 默认 API 端点与客户端标识。
const (
	DefaultAppBaseURL    = "https://api.cloud.189.cn"
	DefaultWebBaseURL    = "https://cloud.189.cn/api"
	DefaultUploadBaseURL = "https://upload.cloud.189.cn"

	AppClientType = "TELEPC"
	AppVersion    = "7.1.8.0"
	AppChannelID  = "web_cloud.189.cn"
	UserAgent     = "desktop"
)

// UploadHost 供签名逻辑判断上传域名。
const UploadHost = "upload.cloud.189.cn"
