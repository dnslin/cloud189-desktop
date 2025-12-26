package cloud189

import (
	"context"
	"errors"
	"path"
	"strconv"
)

// ListOption 配置文件列表参数。
type ListOption func(params map[string]string)

// WithListFileType 设置 fileType。
func WithListFileType(fileType string) ListOption {
	return func(params map[string]string) {
		if fileType != "" {
			params["fileType"] = fileType
		}
	}
}

// WithListPagination 设置分页信息。
func WithListPagination(pageNum, pageSize int) ListOption {
	return func(params map[string]string) {
		if pageNum > 0 {
			params["pageNum"] = strconv.Itoa(pageNum)
		}
		if pageSize > 0 {
			params["pageSize"] = strconv.Itoa(pageSize)
		}
	}
}

// WithListOrder 设置排序字段与顺序。
func WithListOrder(orderBy string, descending bool) ListOption {
	return func(params map[string]string) {
		if orderBy != "" {
			params["orderBy"] = orderBy
		}
		params["descending"] = strconv.FormatBool(descending)
	}
}

// WithListMedia 设置媒体过滤参数。
func WithListMedia(mediaType, mediaAttr string) ListOption {
	return func(params map[string]string) {
		if mediaType != "" {
			params["mediaType"] = mediaType
		}
		if mediaAttr != "" {
			params["mediaAttr"] = mediaAttr
		}
	}
}

// SearchOption 配置搜索参数。
type SearchOption func(params map[string]string)

// WithSearchFolder 指定搜索所在文件夹。
func WithSearchFolder(folderID string) SearchOption {
	return func(params map[string]string) {
		if folderID != "" {
			params["folderId"] = folderID
		}
	}
}

// WithSearchFileType 设置搜索 fileType。
func WithSearchFileType(fileType string) SearchOption {
	return func(params map[string]string) {
		if fileType != "" {
			params["fileType"] = fileType
		}
	}
}

// WithSearchPagination 设置搜索分页。
func WithSearchPagination(pageNum, pageSize int) SearchOption {
	return func(params map[string]string) {
		if pageNum > 0 {
			params["pageNum"] = strconv.Itoa(pageNum)
		}
		if pageSize > 0 {
			params["pageSize"] = strconv.Itoa(pageSize)
		}
	}
}

// WithSearchOrder 设置搜索排序。
func WithSearchOrder(orderBy string, descending bool) SearchOption {
	return func(params map[string]string) {
		if orderBy != "" {
			params["orderBy"] = orderBy
		}
		params["descending"] = strconv.FormatBool(descending)
	}
}

// WithSearchRecursive 开启/关闭递归。
func WithSearchRecursive(recursive bool) SearchOption {
	return func(params map[string]string) {
		if recursive {
			params["recursive"] = "1"
		} else {
			params["recursive"] = "0"
		}
	}
}

// ListFiles 列出指定文件夹内文件与文件夹。
func (c *Client) ListFiles(ctx context.Context, folderID string, opts ...ListOption) (*FileListResponse, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if folderID == "" {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "folderID 不能为空", errors.New("cloud189: folderID 为空"))
	}
	params := map[string]string{
		"folderId":   folderID,
		"fileType":   "0",
		"mediaType":  "0",
		"mediaAttr":  "0",
		"iconOption": "0",
		"orderBy":    "filename",
		"descending": "true",
		"pageNum":    "1",
		"pageSize":   "100",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(params)
		}
	}
	var rsp FileListResponse
	if err := c.AppGet(ctx, "/listFiles.action", params, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

// SearchFiles 搜索文件或文件夹。
func (c *Client) SearchFiles(ctx context.Context, keyword string, opts ...SearchOption) (*SearchResponse, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	params := map[string]string{
		"folderId":   "-11",
		"filename":   keyword,
		"fileType":   "0",
		"mediaType":  "0",
		"mediaAttr":  "0",
		"recursive":  "0",
		"iconOption": "0",
		"orderBy":    "filename",
		"descending": "true",
		"pageNum":    "1",
		"pageSize":   "100",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(params)
		}
	}
	var rsp SearchResponse
	if err := c.AppGet(ctx, "/searchFiles.action", params, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

// CreateFolder 创建文件夹。
func (c *Client) CreateFolder(ctx context.Context, parentID, name string) (*FileInfo, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if name == "" {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "文件夹名不能为空", errors.New("cloud189: 文件夹名不能为空"))
	}
	dir, base := path.Split(name)
	if base == "" {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "文件夹名不能为空", errors.New("cloud189: 文件夹名不能为空"))
	}
	params := map[string]string{
		"folderName":     base,
		"parentFolderId": parentID,
		"relativePath":   dir,
	}
	// relativePath 在 path.Split 返回空字符串时无需携带。
	if dir == "" {
		delete(params, "relativePath")
	}
	var rsp struct {
		CodeResponse
		FileInfo
	}
	if err := c.AppPost(ctx, "/createFolder.action", params, &rsp); err != nil {
		return nil, err
	}
	return &rsp.FileInfo, nil
}

// DeleteFiles 批量删除文件或文件夹。
func (c *Client) DeleteFiles(ctx context.Context, fileIDs []string) error {
	if c == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if len(fileIDs) == 0 {
		return nil
	}
	params := map[string]string{
		"fileIdList": joinIDs(fileIDs),
	}
	var rsp CodeResponse
	return c.AppPost(ctx, "/batchDeleteFile.action", params, &rsp)
}

// CopyFiles 复制文件到目标目录。
func (c *Client) CopyFiles(ctx context.Context, fileIDs []string, destFolderID string) error {
	if c == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if len(fileIDs) == 0 {
		return nil
	}
	for _, id := range fileIDs {
		params := map[string]string{
			"fileId":             id,
			"destParentFolderId": destFolderID,
			"destFileName":       "",
		}
		var rsp CodeResponse
		if err := c.AppPost(ctx, "/copyFile.action", params, &rsp); err != nil {
			return err
		}
	}
	return nil
}

// MoveFiles 移动文件到目标目录。
func (c *Client) MoveFiles(ctx context.Context, fileIDs []string, destFolderID string) error {
	if c == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if len(fileIDs) == 0 {
		return nil
	}
	params := map[string]string{
		"fileIdList":         joinIDs(fileIDs),
		"destParentFolderId": destFolderID,
	}
	var rsp CodeResponse
	return c.AppPost(ctx, "/batchMoveFile.action", params, &rsp)
}

// RenameFile 重命名文件。
func (c *Client) RenameFile(ctx context.Context, fileID, newName string) error {
	if c == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if fileID == "" || newName == "" {
		return WrapCloudError(ErrCodeInvalidRequest, "参数缺失", errors.New("cloud189: fileID 或 newName 为空"))
	}
	params := map[string]string{
		"fileId":       fileID,
		"destFileName": newName,
	}
	var rsp CodeResponse
	return c.AppPost(ctx, "/renameFile.action", params, &rsp)
}

// GetFileInfo 获取文件信息。
func (c *Client) GetFileInfo(ctx context.Context, fileID string) (*FileInfo, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if fileID == "" {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "fileID 不能为空", errors.New("cloud189: fileID 为空"))
	}
	params := map[string]string{
		"fileId":     fileID,
		"filePath":   "",
		"pathList":   "1",
		"iconOption": "0",
	}
	var rsp struct {
		CodeResponse
		FileInfo
	}
	if err := c.AppGet(ctx, "/getFileInfo.action", params, &rsp); err != nil {
		return nil, err
	}
	return &rsp.FileInfo, nil
}

// GetDownloadURL 获取下载链接。
func (c *Client) GetDownloadURL(ctx context.Context, fileID string) (string, error) {
	if c == nil {
		return "", WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if fileID == "" {
		return "", WrapCloudError(ErrCodeInvalidRequest, "fileID 不能为空", errors.New("cloud189: fileID 为空"))
	}
	params := map[string]string{"fileId": fileID}
	var rsp struct {
		CodeResponse
		FileDownloadURL string `json:"fileDownloadUrl,omitempty"`
	}
	if err := c.AppGet(ctx, "/getFileDownloadUrl.action", params, &rsp); err != nil {
		return "", err
	}
	return rsp.FileDownloadURL, nil
}

func joinIDs(ids []string) string {
	switch len(ids) {
	case 0:
		return ""
	case 1:
		return ids[0]
	}
	result := ids[0]
	for i := 1; i < len(ids); i++ {
		result += ";" + ids[i]
	}
	return result
}
