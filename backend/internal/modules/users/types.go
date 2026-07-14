package users

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         *string   `json:"name"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
