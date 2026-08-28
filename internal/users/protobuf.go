package users

import (
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func UserToProto(u User) *pb.User {
	return &pb.User{
		Id:                     uint32(u.ID),
		CreatedAt:              formatDateTime(u.CreatedAt),
		UpdatedAt:              formatDateTime(u.UpdatedAt),
		Email:                  u.Email,
		Role:                   u.Role,
		NotificationsEnabled:   u.NotificationsEnabled,
		NotificationsEnabledAt: formatDateTimePtr(u.NotificationsEnabledAt),
	}
}

func UsersToProto(users []User) []*pb.User {
	result := make([]*pb.User, 0, len(users))
	for _, user := range users {
		result = append(result, UserToProto(user))
	}
	return result
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatDateTimePtr is the *time.Time variant of formatDateTime. It is
// used for nullable timestamps (e.g. Users.NotificationsEnabledAt) where
// a nil pointer maps to an empty string in proto.
func formatDateTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
