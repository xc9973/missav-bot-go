package model

import (
	"time"
)

// SubscriptionType defines the type of subscription
type SubscriptionType string

const (
	SubTypeAll     SubscriptionType = "ALL"
	SubTypeActress SubscriptionType = "ACTRESS"
	SubTypeTag     SubscriptionType = "TAG"
)

// Subscription represents a user's subscription to video updates
type Subscription struct {
	ID        uint             `gorm:"primaryKey"`
	ChatID    int64            `gorm:"index;not null"`
	ChatType  string           `gorm:"size:20"`
	Type      SubscriptionType `gorm:"size:20;not null"`
	Keyword   string           `gorm:"size:100"`
	Enabled   bool             `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the table name for Subscription
func (Subscription) TableName() string {
	return "subscriptions"
}
