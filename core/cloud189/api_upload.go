package cloud189

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultSliceSize 默认分片大小（10MB）。
const DefaultSliceSize = 10 * 1024 * 1024

// UploadSession 记录上传上下文与已上传分片信息。
type UploadSession struct {
	UploadInitData
	ParentID  string
	FileName  string
	FileSize  int64
	SliceSize int
	LazyCheck bool
	Overwrite bool

	FileMD5  string
	SliceMD5 string

	fileMD5    hash.Hash
	partHashes []string
}

type uploadURL struct {
	RequestURL    string `json:"requestURL,omitempty"`
	RequestHeader string `json:"requestHeader,omitempty"`
}

type uploadURLsResponse struct {
	CodeResponse
	UploadURLs map[string]uploadURL `json:"uploadUrls,omitempty"`
}

// InitUpload 初始化分片上传会话。
func (c *Client) InitUpload(ctx context.Context, parentID, filename string, size int64) (*UploadSession, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	if filename == "" {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "文件名不能为空", errors.New("cloud189: 文件名不能为空"))
	}
	params := url.Values{}
	params.Set("parentFolderId", parentID)
	params.Set("fileName", filename)
	if size > 0 {
		params.Set("fileSize", strconv.FormatInt(size, 10))
	}
	params.Set("sliceSize", strconv.Itoa(DefaultSliceSize))
	params.Set("lazyCheck", "1")
	params.Set("extend", `{"opScene":"1","relativepath":"","rootfolderid":""}`)

	var rsp UploadInitResponse
	if err := c.AppUpload(ctx, "/person/initMultiUpload", params, &rsp); err != nil {
		return nil, err
	}
	if rsp.Data.UploadFileID == "" {
		return nil, WrapCloudError(ErrCodeUnknown, "获取 uploadFileId 失败", errors.New("cloud189: uploadFileId 缺失"))
	}
	session := &UploadSession{
		UploadInitData: rsp.Data,
		ParentID:       parentID,
		FileName:       filename,
		FileSize:       size,
		SliceSize:      DefaultSliceSize,
		LazyCheck:      true,
	}
	if rsp.Data.Exists() {
		session.LazyCheck = false
	}
	return session, nil
}

// UploadPart 上传单个分片。
func (c *Client) UploadPart(ctx context.Context, session *UploadSession, partNum int, data io.Reader) error {
	if session == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "上传会话未初始化", errors.New("cloud189: UploadSession 为空"))
	}
	if partNum <= 0 {
		return WrapCloudError(ErrCodeInvalidRequest, "分片序号无效", errors.New("cloud189: 分片序号必须大于 0"))
	}
	if data == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "分片数据为空", errors.New("cloud189: 分片数据为空"))
	}
	if session.UploadFileID == "" {
		return WrapCloudError(ErrCodeInvalidRequest, "uploadFileId 为空", errors.New("cloud189: uploadFileId 未初始化"))
	}
	buf, err := io.ReadAll(data)
	if err != nil {
		return WrapCloudError(ErrCodeUnknown, "读取分片数据失败", err)
	}
	sum := md5.Sum(buf)
	partName := base64.StdEncoding.EncodeToString(sum[:])
	partInfo := fmt.Sprintf("%d-%s", partNum, partName)

	params := url.Values{}
	params.Set("partInfo", partInfo)
	params.Set("uploadFileId", session.UploadFileID)

	var rsp uploadURLsResponse
	if err := c.AppUpload(ctx, "/person/getMultiUploadUrls", params, &rsp); err != nil {
		return err
	}
	key := fmt.Sprintf("partNumber_%d", partNum)
	urlInfo, ok := rsp.UploadURLs[key]
	if !ok {
		return WrapCloudError(ErrCodeInvalidRequest, "上传地址缺失", errors.New("cloud189: 未返回分片上传地址"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, urlInfo.RequestURL, bytes.NewReader(buf))
	if err != nil {
		return WrapCloudError(ErrCodeInvalidRequest, "构建上传请求失败", err)
	}
	for _, h := range strings.Split(urlInfo.RequestHeader, "&") {
		if h == "" {
			continue
		}
		kv := strings.SplitN(h, "=", 2)
		if len(kv) == 2 {
			req.Header.Set(kv[0], kv[1])
		}
	}

	httpClient := http.DefaultClient
	if c != nil && c.http != nil && c.http.HTTP != nil {
		httpClient = c.http.HTTP
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return WrapCloudError(ErrCodeUnknown, "上传分片失败", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return WrapCloudError(ErrCodeServer, fmt.Sprintf("上传失败，状态码=%d", resp.StatusCode), errors.New(resp.Status))
	}
	session.recordHashes(partNum, sum[:], buf)
	return nil
}

// CommitUpload 提交上传，返回文件信息。
func (c *Client) CommitUpload(ctx context.Context, session *UploadSession) (*FileInfo, error) {
	if session == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "上传会话未初始化", errors.New("cloud189: UploadSession 为空"))
	}
	params := url.Values{}
	params.Set("uploadFileId", session.UploadFileID)
	if session.LazyCheck {
		session.computeHashes()
		if session.FileMD5 != "" {
			params.Set("fileMd5", session.FileMD5)
		}
		if session.SliceMD5 != "" {
			params.Set("sliceMd5", session.SliceMD5)
		}
		params.Set("lazyCheck", "1")
	}
	if session.Overwrite {
		params.Set("opertype", "3")
	}

	var rsp UploadCommitResponse
	if err := c.AppUpload(ctx, "/person/commitMultiUploadFile", params, &rsp); err != nil {
		return nil, err
	}
	meta := rsp.File
	return &FileInfo{
		ID:       meta.ID,
		FileName: meta.FileName,
		FileSize: meta.FileSize,
		MD5:      meta.FileMD5,
	}, nil
}

// SimpleUpload 小文件一次性上传。
func (c *Client) SimpleUpload(ctx context.Context, parentID, filename string, data io.Reader) (*FileInfo, error) {
	if data == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "上传数据为空", errors.New("cloud189: 上传数据为空"))
	}
	buf, err := io.ReadAll(data)
	if err != nil {
		return nil, WrapCloudError(ErrCodeUnknown, "读取上传数据失败", err)
	}
	size := int64(len(buf))
	session, err := c.InitUpload(ctx, parentID, filename, size)
	if err != nil {
		return nil, err
	}
	if !session.Exists() {
		if err := c.UploadPart(ctx, session, 1, bytes.NewReader(buf)); err != nil {
			return nil, err
		}
	}
	sum := md5.Sum(buf)
	session.fileMD5 = md5.New()
	session.fileMD5.Write(buf)
	session.FileMD5 = hex.EncodeToString(sum[:])
	session.SliceMD5 = session.FileMD5
	session.recordHashes(1, sum[:], nil)
	return c.CommitUpload(ctx, session)
}

func (s *UploadSession) recordHashes(partNum int, sum []byte, data []byte) {
	if s == nil {
		return
	}
	if s.fileMD5 == nil {
		s.fileMD5 = md5.New()
	}
	if len(data) > 0 {
		s.fileMD5.Write(data)
	}
	if partNum > 0 {
		for len(s.partHashes) < partNum {
			s.partHashes = append(s.partHashes, "")
		}
		s.partHashes[partNum-1] = strings.ToUpper(hex.EncodeToString(sum))
	}
}

func (s *UploadSession) computeHashes() {
	if s == nil {
		return
	}
	if s.FileMD5 == "" && s.fileMD5 != nil {
		s.FileMD5 = hex.EncodeToString(s.fileMD5.Sum(nil))
	}
	if s.SliceMD5 == "" && len(s.partHashes) > 0 {
		if len(s.partHashes) == 1 {
			if s.FileMD5 != "" {
				s.SliceMD5 = s.FileMD5
			} else {
				s.SliceMD5 = strings.ToLower(s.partHashes[0])
			}
			return
		}
		hasher := md5.New()
		joined := strings.Join(s.partHashes, "\n")
		hasher.Write([]byte(joined))
		s.SliceMD5 = hex.EncodeToString(hasher.Sum(nil))
	}
}
