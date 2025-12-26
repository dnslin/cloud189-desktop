package model

import "time"

// File 表示云盘中的文件或文件夹，供业务层使用。
type File struct {
	ID          string
	ParentID    string
	Name        string
	Size        int64
	MD5         string
	MediaType   int
	Category    int
	Revision    string
	Starred     bool
	IsFolder    bool
	ChildCount  int
	ParentPath  string
	DownloadURL string
	UpdatedAt   time.Time
	CreatedAt   time.Time
}
