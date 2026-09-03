package push

import (
	"testing"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func TestCategoryRoundTrip(t *testing.T) {
	cases := []struct {
		domain Category
		proto  pb.PushCategory
	}{
		{CategoryEventAlert, pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT},
		{CategoryApkUpdate, pb.PushCategory_PUSH_CATEGORY_APK_UPDATE},
		{CategoryAdminMessage, pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE},
		{CategoryScheduled, pb.PushCategory_PUSH_CATEGORY_SCHEDULED},
	}
	for _, c := range cases {
		if got := CategoryToProto(c.domain); got != c.proto {
			t.Errorf("CategoryToProto(%q) = %v, want %v", c.domain, got, c.proto)
		}
		if got := CategoryFromProto(c.proto); got != c.domain {
			t.Errorf("CategoryFromProto(%v) = %q, want %q", c.proto, got, c.domain)
		}
	}
}

func TestCategoryValid(t *testing.T) {
	for _, c := range []Category{CategoryEventAlert, CategoryApkUpdate, CategoryAdminMessage, CategoryScheduled} {
		if !c.Valid() {
			t.Errorf("%q.Valid() = false, want true", c)
		}
	}
	if Category("").Valid() {
		t.Error(`"".Valid() = true, want false`)
	}
	if Category("nope").Valid() {
		t.Error(`"nope".Valid() = true, want false`)
	}
}

func TestPriorityRoundTrip(t *testing.T) {
	for _, p := range []Priority{PriorityNormal, PriorityHigh} {
		if got := PriorityToProto(p); got == pb.PushPriority_PUSH_PRIORITY_UNSPECIFIED {
			t.Errorf("PriorityToProto(%q) = UNSPECIFIED", p)
		}
		if got := PriorityFromProto(PriorityToProto(p)); got != p {
			t.Errorf("round trip: %q -> %v -> %q", p, PriorityToProto(p), got)
		}
	}
}

func TestScheduleTypeRoundTrip(t *testing.T) {
	for _, s := range []ScheduleType{ScheduleTypeOneShot, ScheduleTypeRecurring} {
		if got := ScheduleTypeToProto(s); got == pb.PushScheduleType_PUSH_SCHEDULE_TYPE_UNSPECIFIED {
			t.Errorf("ScheduleTypeToProto(%q) = UNSPECIFIED", s)
		}
		if got := ScheduleTypeFromProto(ScheduleTypeToProto(s)); got != s {
			t.Errorf("round trip: %q -> %v -> %q", s, ScheduleTypeToProto(s), got)
		}
	}
}

func TestPayloadFromProto_RejectsUnspecified(t *testing.T) {
	p := &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_UNSPECIFIED,
		Title:    "t",
		Body:     "b",
		Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}
	if _, _, _, _, _, _, _, ok := PayloadFromProto(p); ok {
		t.Fatal("expected PayloadFromProto to reject UNSPECIFIED category")
	}
}

func TestPayloadFromProto_RejectsBadTTL(t *testing.T) {
	p := &pb.PushPayload{
		Category:   pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
		Title:      "t",
		Body:       "b",
		Priority:   pb.PushPriority_PUSH_PRIORITY_NORMAL,
		TtlSeconds: -1,
	}
	if _, _, _, _, _, _, _, ok := PayloadFromProto(p); ok {
		t.Fatal("expected PayloadFromProto to reject negative TTL")
	}
}

func TestPayloadFromProto_AcceptsValid(t *testing.T) {
	p := &pb.PushPayload{
		Category:   pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
		Title:      "Goal!",
		Body:       "Real Madrid 1 - 0 Barcelona",
		ImageUrl:   "https://cdn.example.com/img.png",
		Priority:   pb.PushPriority_PUSH_PRIORITY_HIGH,
		TtlSeconds: 300,
		Data:       map[string]string{"match_id": "123"},
	}
	category, title, body, image, priority, ttl, data, ok := PayloadFromProto(p)
	if !ok {
		t.Fatal("expected ok=true for valid payload")
	}
	if category != CategoryEventAlert {
		t.Errorf("category = %q, want event_alert", category)
	}
	if title != "Goal!" {
		t.Errorf("title = %q", title)
	}
	if body != "Real Madrid 1 - 0 Barcelona" {
		t.Errorf("body = %q", body)
	}
	if image != "https://cdn.example.com/img.png" {
		t.Errorf("image = %q", image)
	}
	if priority != PriorityHigh {
		t.Errorf("priority = %q", priority)
	}
	if ttl != 300 {
		t.Errorf("ttl = %d", ttl)
	}
	if data["match_id"] != "123" {
		t.Errorf("data[match_id] = %q", data["match_id"])
	}
}

func TestDeliveryStateValid(t *testing.T) {
	for _, s := range []DeliveryState{StateSent, StateDelivered, StateFailed} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false, want true", s)
		}
	}
	if DeliveryState("bogus").Valid() {
		t.Error(`"bogus".Valid() = true, want false`)
	}
}

func TestStringJSONValueAndScan(t *testing.T) {
	// nil map -> SQL NULL
	v, err := StringJSON(nil).Value()
	if err != nil {
		t.Fatalf("Value(nil): %v", err)
	}
	if v != nil {
		t.Errorf("Value(nil) = %v, want nil (SQL NULL)", v)
	}

	// non-nil map -> JSON bytes
	v, err = StringJSON{"a": "1", "b": "2"}.Value()
	if err != nil {
		t.Fatalf("Value(map): %v", err)
	}
	if v == nil {
		t.Fatal("Value(map) = nil, want JSON bytes")
	}

	// Round trip via Scan([]byte)
	var got StringJSON
	if err := got.Scan([]byte(`{"a":"1","b":"2"}`)); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("Scan got = %v, want {a:1, b:2}", got)
	}

	// Scan of empty bytes -> nil
	var nilMap StringJSON
	if err := nilMap.Scan([]byte{}); err != nil {
		t.Fatalf("Scan(empty): %v", err)
	}
	if nilMap != nil {
		t.Errorf("Scan(empty) = %v, want nil", nilMap)
	}

	// Scan of nil -> nil
	if err := nilMap.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if nilMap != nil {
		t.Errorf("Scan(nil) = %v, want nil", nilMap)
	}
}
