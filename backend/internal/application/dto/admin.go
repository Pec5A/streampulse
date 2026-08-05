package dto

// StatsResponse is the admin dashboard snapshot. Scoped to the users table
// (the only one this slice owns); extend as other features' tables land.
type StatsResponse struct {
	TotalUsers        int `json:"total_users"`
	TotalRegular      int `json:"total_regular"`
	TotalBroadcasters int `json:"total_broadcasters"`
	TotalAdmins       int `json:"total_admins"`
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}
