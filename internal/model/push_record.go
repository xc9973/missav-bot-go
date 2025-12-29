package model

import (
	"time"
)

// PushStatus defines the status of a push operation
type PushStatus string

const (
	PushStatusSuccess PushStatus = "SUCCESS"
	PushStatusFailed  PushStatus = "FAILED"
)

// PushRecord represents a record of a video push to a chat
type PushRecord struct {
	ID         uint       `gorm:"primaryKey"`
	VideoID    uint       `gorm:"index;not null"`
	ChatID     int64      `gorm:"index;not null"`
	Status     PushStatus `gorm:"size:20;not null"`
	FailReason string     `gorm:"size:500"`
	MessageID  int
	PushedAt   time.Time
	CreatedAt  time.Time
}

// TableName returns the table name for PushRecord
func (PushRecord) TableName() string {
	return "push_records"
}
