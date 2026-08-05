package entity

import "time"

// Role gates access to broadcaster/admin-only actions across the app.
type Role string

const (
	RoleUser        Role = "user"
	RoleBroadcaster Role = "broadcaster"
	RoleAdmin       Role = "admin"
)

type User struct {
	ID        string
	Email     string
	Username  string
	Password  string // bcrypt hash — never serialized to clients
	Role      Role
	CreatedAt time.Time
	UpdatedAt time.Time
}
