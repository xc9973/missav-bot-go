package model

import (
	"time"
)

// Video represents a video entity with metadata
type Video struct {
	ID          uint       `gorm:"primaryKey"`
	Code        string     `gorm:"uniqueIndex;size:50;not null"`
	Title       string     `gorm:"size:500"`
	Actresses   string     `gorm:"size:500"`
	Tags        string     `gorm:"size:500"`
	Duration    int        `gorm:"default:0"`
	ReleaseDate *time.Time `gorm:"type:date"`
	CoverURL    string     `gorm:"size:500"`
	PreviewURL  string     `gorm:"size:500"`
	DetailURL   string     `gorm:"size:500"`
	Pushed      bool       `gorm:"default:false;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName returns the table name for Video
func (Video) TableName() string {
	return "videos"
}
