package cloud189

import (
	"strconv"
	"strings"
	"time"

	"github.com/dnslin/cloud189-desktop/core/model"
)

// CloudTime 兼容时间戳字符串与毫秒整数。
type CloudTime struct {
	time.Time
}

func (t *CloudTime) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), "\"")
	if raw == "" || raw == "null" {
		return nil
	}
	if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if ts > 1e12 { // 毫秒级时间戳（大于 2001-09-09）
			t.Time = time.UnixMilli(ts)
		} else {
			t.Time = time.Unix(ts, 0)
		}
		return nil
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", raw)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// UserInfo 汇总用户空间与会话信息。
type UserInfo struct {
	CodeResponse
	UserID      string `json:"userId,omitempty"`
	UserName    string `json:"userName,omitempty"`
	NickName    string `json:"nickName,omitempty"`
	FamilyID    string `json:"familyId,omitempty"`
	SessionKey  string `json:"sessionKey,omitempty"`
	Capacity    uint64 `json:"capacity,omitempty"`
	Available   uint64 `json:"available,omitempty"`
	UsedSize    uint64 `json:"usedSize,omitempty"`
	BackupSpace uint64 `json:"backupCapacity,omitempty"`
}

// FileInfo 统一 App/Web 文件或文件夹描述。
type FileInfo struct {
	ID            FlexString `json:"id,omitempty"`
	ParentID      FlexString `json:"parentId,omitempty"`
	FileName      string     `json:"name,omitempty"`
	FileSize      int64      `json:"size,omitempty"`
	MD5           string     `json:"md5,omitempty"`
	MediaType     int        `json:"mediaType,omitempty"`
	FileCategory  int        `json:"fileCata,omitempty"`
	Orientation   int        `json:"orientation,omitempty"`
	Rev           FlexString `json:"rev,omitempty"`
	StarLabel     int        `json:"starLabel,omitempty"`
	LastOpTime    CloudTime  `json:"lastOpTime,omitempty"`
	CreateDate    CloudTime  `json:"createDate,omitempty"`
	IsFolder      bool       `json:"isFolder,omitempty"`
	FileCount     int        `json:"fileCount,omitempty"`
	FileListSize  int        `json:"fileListSize,omitempty"`
	ParentPath    string     `json:"filePath,omitempty"`
	DownloadURL   string     `json:"fileDownloadUrl,omitempty"`
	IconLargeURL  string     `json:"largeUrl,omitempty"`
	IconMediumURL string     `json:"mediumUrl,omitempty"`
	IconSmallURL  string     `json:"smallUrl,omitempty"`
}

// FileListResult 表示列表接口中的文件与文件夹集合。
type FileListResult struct {
	Count        int        `json:"count,omitempty"`
	FileListSize int        `json:"fileListSize,omitempty"`
	Files        []FileInfo `json:"fileList,omitempty"`
	Folders      []FileInfo `json:"folderList,omitempty"`
}

// Items 合并文件与文件夹，统一标记文件夹标识。
func (r FileListResult) Items() []FileInfo {
	var items []FileInfo
	for _, folder := range r.Folders {
		copied := folder
		copied.IsFolder = true
		items = append(items, copied)
	}
	items = append(items, r.Files...)
	return items
}

// FileListResponse 兼容 App/Web 的文件列表响应。
type FileListResponse struct {
	CodeResponse
	FileListAO  FileListResult `json:"fileListAO,omitempty"`
	Data        []FileInfo     `json:"data,omitempty"`
	RecordCount int            `json:"recordCount,omitempty"`
	LastRev     int64          `json:"lastRev,omitempty"`
}

// Items 返回聚合后的文件列表。
func (r FileListResponse) Items() []FileInfo {
	items := r.FileListAO.Items()
	if len(r.Data) > 0 {
		items = append(items, r.Data...)
	}
	return items
}

// SearchResponse 兼容 App/Web 的搜索结果。
type SearchResponse struct {
	CodeResponse
	Count   int        `json:"count,omitempty"`
	Files   []FileInfo `json:"fileList,omitempty"`
	Folders []FileInfo `json:"folderList,omitempty"`
}

// Items 返回搜索到的所有文件与文件夹。
func (r SearchResponse) Items() []FileInfo {
	var items []FileInfo
	for _, folder := range r.Folders {
		copied := folder
		copied.IsFolder = true
		items = append(items, copied)
	}
	items = append(items, r.Files...)
	return items
}

// CapacityInfo 描述用户空间容量。
type CapacityInfo struct {
	CodeResponse
	Capacity    uint64 `json:"capacity,omitempty"`
	Available   uint64 `json:"available,omitempty"`
	UsedSize    uint64 `json:"usedSize,omitempty"`
	BackupSpace uint64 `json:"backupCapacity,omitempty"`
}

// SignInResult 记录签到结果与奖励信息。
type SignInResult struct {
	CodeResponse
	Result    int    `json:"result,omitempty"`
	ResultTip string `json:"resultTip,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	PrizeName string `json:"prizeName,omitempty"`
}

// UploadInitData 描述分片上传初始化结果。
type UploadInitData struct {
	UploadType     int    `json:"uploadType,omitempty"`
	UploadHost     string `json:"uploadHost,omitempty"`
	UploadFileID   string `json:"uploadFileId,omitempty"`
	FileDataExists int    `json:"fileDataExists,omitempty"`
}

// Exists 标记服务器是否已有文件数据。
func (d UploadInitData) Exists() bool {
	return d.FileDataExists == 1
}

// UploadInitResponse 分片上传初始化响应。
type UploadInitResponse struct {
	CodeResponse
	Data UploadInitData `json:"data,omitempty"`
}

// UploadFileMeta 上传完成返回的文件元信息。
type UploadFileMeta struct {
	ID         string `json:"userFileId,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileMD5    string `json:"file_md_5,omitempty"`
	CreateDate string `json:"create_date,omitempty"`
}

// UploadCommitResponse 分片上传提交响应。
type UploadCommitResponse struct {
	CodeResponse
	File UploadFileMeta `json:"file,omitempty"`
}

// ToModel 将文件信息转换为领域模型。
func (f FileInfo) ToModel() model.File {
	return model.File{
		ID:          f.ID.String(),
		ParentID:    f.ParentID.String(),
		Name:        f.FileName,
		Size:        f.FileSize,
		MD5:         f.MD5,
		MediaType:   f.MediaType,
		Category:    f.FileCategory,
		Revision:    f.Rev.String(),
		Starred:     f.StarLabel > 0,
		IsFolder:    f.IsFolder,
		ChildCount:  f.FileCount,
		ParentPath:  f.ParentPath,
		DownloadURL: f.DownloadURL,
		UpdatedAt:   f.LastOpTime.Time,
		CreatedAt:   f.CreateDate.Time,
	}
}

// ToModel 将用户信息转换为领域模型。
func (u UserInfo) ToModel() model.User {
	return model.User{
		ID:         u.UserID,
		Name:       u.UserName,
		NickName:   u.NickName,
		FamilyID:   u.FamilyID,
		SessionKey: u.SessionKey,
		Quota: model.StorageQuota{
			Capacity:  u.Capacity,
			Available: u.Available,
			Used:      u.UsedSize,
			Backup:    u.BackupSpace,
		},
	}
}

// ToModel 将容量信息转换为领域模型。
func (c CapacityInfo) ToModel() model.StorageQuota {
	return model.StorageQuota{
		Capacity:  c.Capacity,
		Available: c.Available,
		Used:      c.UsedSize,
		Backup:    c.BackupSpace,
	}
}
