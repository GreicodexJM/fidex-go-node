package domain

import (
	"context"
	"errors"
	"time"
)

// ErrDuplicateMessageID is returned by MessageRepository.Create when the
// underlying store rejects an insert because a row with the same
// message_id already exists. Spec §9.3 requires callers to translate this
// into a deterministic 409 Conflict response to preserve idempotent
// replay semantics — never let it surface as a 500.
var ErrDuplicateMessageID = errors.New("duplicate message_id")

// MessageStatus represents the status of a message in the system
type MessageStatus string

const (
	StatusQueued      MessageStatus = "QUEUED"
	StatusSent        MessageStatus = "SENT"
	StatusDelivered   MessageStatus = "DELIVERED"
	StatusFailed      MessageStatus = "FAILED"
	StatusQuarantined MessageStatus = "QUARANTINED"
)

// MessageDirection represents whether a message is inbound or outbound
type MessageDirection string

const (
	DirectionInbound  MessageDirection = "INBOUND"
	DirectionOutbound MessageDirection = "OUTBOUND"
)

// Message represents a FideX AS5 message
type Message struct {
	ID          int64            `json:"id"`
	MessageID   string           `json:"message_id"`
	Direction   MessageDirection `json:"direction"`
	Status      MessageStatus    `json:"status"`
	Payload     string           `json:"payload"`
	RetryCount  int              `json:"retry_count"`
	NextRetryAt *time.Time       `json:"next_retry_at"`
	LastError   string           `json:"last_error"`
	CreatedAt   time.Time        `json:"created_at"`
}

// Partner represents a trading partner connection profile
type Partner struct {
	ID                 int64      `json:"id"`
	PartnerID          string     `json:"partner_id"`
	Name               string     `json:"name"`
	JWKSUrl            string     `json:"jwks_url"`
	MessageEndpoint    string     `json:"message_endpoint"`
	MDNReceiptEndpoint string     `json:"mdn_receipt_endpoint"`
	PublicKeyJWKS      string     `json:"public_key_jwks"`
	LastKeyRefresh     *time.Time `json:"last_key_refresh"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// User represents a dashboard user
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // Never expose password hash in JSON
	CreatedAt    time.Time `json:"created_at"`
}

// Session represents a user session
type Session struct {
	SessionID string    `json:"session_id"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageRepository defines the interface for message persistence operations
type MessageRepository interface {
	// Create creates a new message in the repository
	Create(ctx context.Context, msg *Message) error

	// GetByID retrieves a message by its message_id
	GetByID(ctx context.Context, messageID string) (*Message, error)

	// ListByStatus retrieves all messages with the specified status
	ListByStatus(ctx context.Context, status MessageStatus) ([]*Message, error)

	// ListPaginated returns messages ordered by created_at DESC.
	// If statusFilter is non-nil, only messages with that status are returned.
	// total is the count of rows matching the filter (ignoring limit/offset).
	ListPaginated(ctx context.Context, statusFilter *MessageStatus, limit, offset int) (messages []*Message, total int, err error)

	// CountByStatusSince returns a per-status count of messages created at or
	// after the given instant. Statuses with zero matches are omitted from
	// the result map.
	CountByStatusSince(ctx context.Context, since time.Time) (map[MessageStatus]int, error)

	// UpdateStatus updates the status of a message
	UpdateStatus(ctx context.Context, messageID string, status MessageStatus) error

	// UpdateRetryInfo updates the retry information for a message
	UpdateRetryInfo(ctx context.Context, messageID string, retryCount int, nextRetryAt *time.Time, lastError string) error

	// Delete removes a message
	Delete(ctx context.Context, messageID string) error
}

// PartnerRepository defines the interface for trading partner persistence operations
type PartnerRepository interface {
	// Create creates a new trading partner
	Create(ctx context.Context, partner *Partner) error

	// GetByID retrieves a partner by their partner_id
	GetByID(ctx context.Context, partnerID string) (*Partner, error)

	// Update updates an existing trading partner
	Update(ctx context.Context, partner *Partner) error

	// Upsert inserts a partner or, on partner_id conflict, updates the
	// existing row's name / jwks_url / public_key_jwks / last_key_refresh /
	// updated_at. Used by the discovery handler.
	Upsert(ctx context.Context, partner *Partner) error

	// Delete removes a trading partner by partner_id
	Delete(ctx context.Context, partnerID string) error

	// DeleteByDBID removes a trading partner by its database numeric id.
	// Used by the dashboard settings UI which addresses partners by row id.
	DeleteByDBID(ctx context.Context, id int64) error

	// UpdateNameByDBID renames a partner identified by its database numeric id.
	UpdateNameByDBID(ctx context.Context, id int64, name string) error

	// List retrieves all trading partners
	List(ctx context.Context) ([]*Partner, error)

	// Count returns the total number of registered partners.
	Count(ctx context.Context) (int, error)
}

// UserRepository defines the interface for user persistence operations
type UserRepository interface {
	// Create creates a new user
	Create(ctx context.Context, username, passwordHash string) (*User, error)

	// GetByID retrieves a user by ID
	GetByID(ctx context.Context, userID int64) (*User, error)

	// GetByUsername retrieves a user by username
	GetByUsername(ctx context.Context, username string) (*User, error)

	// List retrieves all users
	List(ctx context.Context) ([]*User, error)

	// Delete removes a user
	Delete(ctx context.Context, userID int64) error

	// UpdatePassword updates a user's password hash
	UpdatePassword(ctx context.Context, userID int64, newPasswordHash string) error
}

// SessionRepository defines the interface for session persistence operations
type SessionRepository interface {
	// Create creates a new session
	Create(ctx context.Context, session *Session) error

	// GetByID retrieves a session by session_id
	GetByID(ctx context.Context, sessionID string) (*Session, error)

	// Delete removes a session
	Delete(ctx context.Context, sessionID string) error

	// DeleteByUserID removes all sessions belonging to the given user.
	// Used when the user account is deleted.
	DeleteByUserID(ctx context.Context, userID int64) error

	// DeleteExpired removes all expired sessions
	DeleteExpired(ctx context.Context) error
}
