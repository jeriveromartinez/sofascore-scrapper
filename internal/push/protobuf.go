package push

import (
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// CategoryToProto maps a domain Category to the proto enum.
func CategoryToProto(c Category) pb.PushCategory {
	switch c {
	case CategoryEventAlert:
		return pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT
	case CategoryApkUpdate:
		return pb.PushCategory_PUSH_CATEGORY_APK_UPDATE
	case CategoryAdminMessage:
		return pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE
	case CategoryScheduled:
		return pb.PushCategory_PUSH_CATEGORY_SCHEDULED
	}
	return pb.PushCategory_PUSH_CATEGORY_UNSPECIFIED
}

// CategoryFromProto maps a proto enum to the domain Category. Unknown
// values return "" so the caller can decide whether to reject or
// store-as-is.
func CategoryFromProto(c pb.PushCategory) Category {
	switch c {
	case pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT:
		return CategoryEventAlert
	case pb.PushCategory_PUSH_CATEGORY_APK_UPDATE:
		return CategoryApkUpdate
	case pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE:
		return CategoryAdminMessage
	case pb.PushCategory_PUSH_CATEGORY_SCHEDULED:
		return CategoryScheduled
	}
	return ""
}

// PriorityToProto maps a domain Priority to the proto enum.
func PriorityToProto(p Priority) pb.PushPriority {
	switch p {
	case PriorityHigh:
		return pb.PushPriority_PUSH_PRIORITY_HIGH
	case PriorityNormal:
		return pb.PushPriority_PUSH_PRIORITY_NORMAL
	}
	return pb.PushPriority_PUSH_PRIORITY_UNSPECIFIED
}

// PriorityFromProto maps a proto enum to the domain Priority.
func PriorityFromProto(p pb.PushPriority) Priority {
	switch p {
	case pb.PushPriority_PUSH_PRIORITY_HIGH:
		return PriorityHigh
	case pb.PushPriority_PUSH_PRIORITY_NORMAL:
		return PriorityNormal
	}
	return ""
}

// ScheduleTypeToProto maps a domain ScheduleType to the proto enum.
func ScheduleTypeToProto(s ScheduleType) pb.PushScheduleType {
	switch s {
	case ScheduleTypeOneShot:
		return pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT
	case ScheduleTypeRecurring:
		return pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING
	}
	return pb.PushScheduleType_PUSH_SCHEDULE_TYPE_UNSPECIFIED
}

// ScheduleTypeFromProto maps a proto enum to the domain ScheduleType.
func ScheduleTypeFromProto(s pb.PushScheduleType) ScheduleType {
	switch s {
	case pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT:
		return ScheduleTypeOneShot
	case pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING:
		return ScheduleTypeRecurring
	}
	return ""
}

// PayloadToProto converts a (category, title, body, ...) tuple to a
// pb.PushPayload. This is the canonical proto representation; both
// CreateImmediatePushRequest and CreateScheduleRequest use it.
func PayloadToProto(category Category, title, body, imageURL, deepLink string, priority Priority, ttlSeconds int, data StringJSON) *pb.PushPayload {
	return &pb.PushPayload{
		Category:   CategoryToProto(category),
		Title:      title,
		Body:       body,
		ImageUrl:   imageURL,
		DeepLink:   deepLink,
		Priority:   PriorityToProto(priority),
		TtlSeconds: int32(ttlSeconds),
		Data:       map[string]string(data),
	}
}

// PayloadFromProto is the inverse of PayloadToProto. Returns ok=false
// when the category or priority is UNSPECIFIED so the caller can
// reject the request with a 400.
func PayloadFromProto(p *pb.PushPayload) (category Category, title, body, imageURL, deepLink string, priority Priority, ttlSeconds int, data StringJSON, ok bool) {
	if p == nil {
		return "", "", "", "", "", "", 0, nil, false
	}
	category = CategoryFromProto(p.Category)
	if category == "" {
		return "", "", "", "", "", "", 0, nil, false
	}
	priority = PriorityFromProto(p.Priority)
	if priority == "" {
		return "", "", "", "", "", "", 0, nil, false
	}
	if p.TtlSeconds < 0 || p.TtlSeconds > 86400 {
		return "", "", "", "", "", "", 0, nil, false
	}
	return category, p.Title, p.Body, p.ImageUrl, p.DeepLink, priority, int(p.TtlSeconds), StringJSON(p.Data), true
}

// PushMessageToProto converts a domain PushMessage to a pb.PushMessage
// for the REST response. Domains are flattened to a list of IDs; the
// caller is expected to preload the Domains association before
// calling this helper.
func PushMessageToProto(m PushMessage) *pb.PushMessage {
	domainIDs := make([]uint32, 0, len(m.Domains))
	for _, d := range m.Domains {
		domainIDs = append(domainIDs, uint32(d.ID))
	}
	out := &pb.PushMessage{
		Id:         uint32(m.ID),
		CreatedAt:  formatDateTime(m.CreatedAt),
		UserId:     uint32(m.UserID),
		Category:   CategoryToProto(m.Category),
		Title:      m.Title,
		Body:       m.Body,
		ImageUrl:   m.ImageURL,
		DeepLink:   m.DeepLink,
		Priority:   PriorityToProto(m.Priority),
		TtlSeconds: int32(m.TTLSeconds),
		Data:       map[string]string(m.DataJSON),
		Source:     string(m.Source),
		DomainIds:  domainIDs,
	}
	if m.ScheduledID != nil {
		out.ScheduledId = uint32(*m.ScheduledID)
	}
	return out
}

// ScheduledPushToProto converts a domain ScheduledPush to a
// pb.ScheduledPush. Domains must be preloaded.
func ScheduledPushToProto(s ScheduledPush) *pb.ScheduledPush {
	domainIDs := make([]uint32, 0, len(s.Domains))
	for _, d := range s.Domains {
		domainIDs = append(domainIDs, uint32(d.ID))
	}
	out := &pb.ScheduledPush{
		Id:           uint32(s.ID),
		CreatedAt:    formatDateTime(s.CreatedAt),
		UpdatedAt:    formatDateTime(s.UpdatedAt),
		UserId:       uint32(s.UserID),
		ScheduleType: ScheduleTypeToProto(s.ScheduleType),
		NextFireAt:   formatDateTime(s.NextFireAt),
		IsActive:     s.IsActive,
		DomainIds:    domainIDs,
		Payload: &pb.PushPayload{
			Category:   CategoryToProto(s.Category),
			Title:      s.Title,
			Body:       s.Body,
			ImageUrl:   s.ImageURL,
			DeepLink:   s.DeepLink,
			Priority:   PriorityToProto(s.Priority),
			TtlSeconds: int32(s.TTLSeconds),
			Data:       map[string]string(s.DataJSON),
		},
	}
	if s.RunAt != nil {
		out.RunAt = formatDateTime(*s.RunAt)
	}
	if s.CronExpr != "" {
		out.CronExpr = s.CronExpr
	}
	if s.LastFiredAt != nil {
		out.LastFiredAt = formatDateTime(*s.LastFiredAt)
	}
	return out
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func formatDateTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
