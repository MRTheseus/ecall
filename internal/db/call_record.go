package db

import (
	"time"

	"gorm.io/gorm"
)

// CallRecord 通话记录表
type CallRecord struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	SessionID     string     `gorm:"column:session_id;index" json:"session_id"`
	DeviceID      string     `gorm:"column:device_id;index" json:"device_id"`
	RemoteNumber  string     `gorm:"column:remote_number;index" json:"remote_number"`
	Direction     string     `gorm:"column:direction" json:"direction"` // "inbound" | "outbound"
	State         string     `gorm:"column:state" json:"state"`         // "completed" (接通), "missed" (未接), "busy" (拒接), "canceled" (取消)
	DurationSec   int        `gorm:"column:duration_sec" json:"duration_sec"`
	HangupReason  string     `gorm:"column:hangup_reason" json:"hangup_reason"`
	HasRecording  bool       `gorm:"column:has_recording;default:false" json:"has_recording"`
	RecordingFile string     `gorm:"column:recording_file" json:"recording_file,omitempty"`
	RecordingSize int64      `gorm:"column:recording_size" json:"recording_size,omitempty"`
	StartedAt     time.Time  `gorm:"column:started_at;index:idx_call_records_started_at,sort:desc" json:"started_at"`
	ConnectedAt   *time.Time `gorm:"column:connected_at" json:"connected_at,omitempty"`
	EndedAt       time.Time  `gorm:"column:ended_at" json:"ended_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (CallRecord) TableName() string { return "call_records" }

type TopCallContact struct {
	RemoteNumber string `json:"remote_number"`
	CallCount    int    `json:"call_count"`
	LastCallAt   string `json:"last_call_at"`
}

// SaveCallRecord 保存通话记录
func SaveCallRecord(record *CallRecord) error {
	if DB == nil {
		return nil
	}
	return DB.Create(record).Error
}

// GetCallRecordByID 根据 ID 获取单条通话记录
func GetCallRecordByID(id uint) (*CallRecord, error) {
	if DB == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var record CallRecord
	if err := DB.First(&record, id).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateCallRecordRecording 更新通话记录的录音文件信息
func UpdateCallRecordRecording(sessionID string, file string, size int64) error {
	if DB == nil || sessionID == "" {
		return nil
	}
	return DB.Model(&CallRecord{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"has_recording":  true,
			"recording_file": file,
			"recording_size": size,
		}).Error
}

// GetCallRecords 获取通话记录列表（支持设备过滤、按时间倒序）
func GetCallRecords(deviceID string, limit int, offset int) ([]CallRecord, int64, error) {
	if DB == nil {
		return nil, 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	var records []CallRecord
	var total int64

	query := DB.Model(&CallRecord{})
	if deviceID != "" && deviceID != "all" {
		query = query.Where("device_id = ?", deviceID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

// GetTopCallContacts 统计近期通话最频繁的联系人 (默认前 3 个)
func GetTopCallContacts(limit int) ([]TopCallContact, error) {
	if DB == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}

	var results []TopCallContact
	err := DB.Model(&CallRecord{}).
		Select("remote_number, count(*) as call_count, max(started_at) as last_call_at").
		Where("remote_number != ''").
		Group("remote_number").
		Order("call_count DESC, last_call_at DESC").
		Limit(limit).
		Scan(&results).Error

	return results, err
}

// DeleteCallRecord 删除单条通话记录
func DeleteCallRecord(id uint) error {
	if DB == nil {
		return nil
	}
	return DB.Delete(&CallRecord{}, id).Error
}

// ClearCallRecords 清空所有通话记录
func ClearCallRecords() error {
	if DB == nil {
		return nil
	}
	return DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&CallRecord{}).Error
}
