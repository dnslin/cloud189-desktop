package model

// StorageQuota 描述用户存储配额。
type StorageQuota struct {
	Capacity  uint64
	Available uint64
	Used      uint64
	Backup    uint64
}

// User 描述登录用户信息。
type User struct {
	ID       string
	Name     string
	NickName string
	FamilyID string
	Quota    StorageQuota
}
