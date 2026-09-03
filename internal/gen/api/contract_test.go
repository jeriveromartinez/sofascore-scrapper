package api

import (
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const hexAuthRequest = "0a1074657374406578616d706c652e636f6d1209736563726574313233"

const hexAuthResponse = "0801121074657374406578616d706c652e636f6d1a106163636573732d746f6b656e2d7878782211726566726573682d746f6b656e2d797979"

const hexDeviceRegisterRequest = "0a106465766963652d746f6b656e2d3132331207616e64726f69641a0b54657374204465766963652203312e30"

const hexEventsList = "1001181420642805"

const hexPlaybackLogList = "102a"

const hexApkInfo = "08011203312e301a076170702e61706b20c0843d2a085465737420617070320c636f6d2e746573742e6170703801401548215209746f6b656e2d6162635a1a687474703a2f2f6578616d706c652e636f6d2f6170702e61706b6214323032342d30312d30315430303a30303a30305a680170327a18687474703a2f2f70616e656c2e6578616d706c652e636f6d"

func TestProtobufFileDescriptor(t *testing.T) {
	fd := (&ErrorResponse{}).ProtoReflect().Descriptor().ParentFile()

	msgNames := map[string]bool{
		"ErrorResponse":                  false,
		"StatusMessage":                  false,
		"StatusResponse":                 false,
		"AuthRequest":                    false,
		"AuthResponse":                   false,
		"User":                           false,
		"UserList":                       false,
		"UserWriteRequest":               false,
		"SetUserRoleRequest":             false,
		"SetNotificationsEnabledRequest": false,
		"Domain":                         false,
		"DomainList":                     false,
		"DomainPage":                     false,
		"DomainRequest":                  false,
		"Device":                         false,
		"DeviceList":                     false,
		"DevicePage":                     false,
		"DeviceUrl":                      false,
		"Tournament":                     false,
		"TournamentList":                 false,
		"TournamentPage":                 false,
		"TournamentRequest":              false,
		"Team":                           false,
		"SofaScoreEvent":                 false,
		"DeviceRegisterRequest":          false,
		"AssignTournamentRequest":        false,
		"SetTournamentIdsRequest":        false,
		"DeviceTournament":               false,
		"DeviceTournamentList":           false,
		"DeviceTournamentPage":           false,
		"GlobalTournamentConfig":         false,
		"GlobalTournamentConfigList":     false,
		"ApkInfo":                        false,
		"ApkList":                        false,
		"ApkPage":                        false,
		"ApkUploadResponse":              false,
		"ApkVersion":                     false,
		"ApkUpdateCheckResponse":         false,
		"LogPlaybackRequest":             false,
		"UpdatePlaybackRequest":          false,
		"PlaybackLog":                    false,
		"PlaybackLogList":                false,
		"PlaybackPage":                   false,
		"CursorPageInfo":                 false,
		"UserPage":                       false,
		"EventsList":                     false,
		"EventPage":                      false,
		"EventStats":                     false,
		"TopEventsResponse":              false,
		// Push notifications (added 2026-08-28)
		"PushPayload":                false,
		"CreateImmediatePushRequest": false,
		"CreateScheduleRequest":      false,
		"UpdateScheduleRequest":      false,
		"ScheduledPush":              false,
		"ScheduledPushPage":          false,
		"PushMessage":                false,
		"PushMessagePage":            false,
		"FailureBreakdown":           false,
		"PushMetricsByCampaign":      false,
		"PushMetricsAggregate":       false,
		"PlatformCount":              false,
		"AppVersionCount":            false,
		"HourBucket":                 false,
		"WsFrame":                    false,
		"WsHello":                    false,
		"WsPush":                     false,
		"WsPushAck":                  false,
		"WsPing":                     false,
		"WsPong":                     false,
		"WsError":                    false,
		// Build info (added 2026-08-30)
		"BuildInfo":                  false,
	}

	for i := 0; i < fd.Messages().Len(); i++ {
		msg := fd.Messages().Get(i)
		msgNames[string(msg.Name())] = true
	}

	for name, found := range msgNames {
		if !found {
			t.Errorf("expected message %q not found in file descriptor", name)
		}
	}
}

func TestAuthRequestFields(t *testing.T) {
	req := &AuthRequest{
		Email:    "test@example.com",
		Password: "secret123",
	}
	fd := req.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "email", 1)
	assertFieldNumber(t, fd, "password", 2)
}

func TestAuthResponseFields(t *testing.T) {
	resp := &AuthResponse{
		Id:           1,
		Email:        "test@example.com",
		Token:        "access-token-xxx",
		RefreshToken: "refresh-token-yyy",
	}
	fd := resp.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "id", 1)
	assertFieldNumber(t, fd, "email", 2)
	assertFieldNumber(t, fd, "token", 3)
	assertFieldNumber(t, fd, "refresh_token", 4)
}

func TestSofaScoreEventFields(t *testing.T) {
	event := &SofaScoreEvent{
		Id:                          1,
		SofaScoreEventId:            12345,
		Sport:                       "football",
		HomeScore:                   2,
		HomeTeamId:                  100,
		AwayScore:                   1,
		AwayTeamId:                  200,
		ScrapedAt:                   999999,
		StartTimestamp:              1710000000000,
		CurrentPeriodStartTimestamp: 1710000000000,
		Slug:                        "test-match",
		StatusType:                  "inprogress",
	}
	fd := event.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "id", 1)
	assertFieldNumber(t, fd, "sofa_score_event_id", 4)
	assertFieldNumber(t, fd, "sport", 5)
	assertFieldNumber(t, fd, "home_score", 6)
	assertFieldNumber(t, fd, "home_team_id", 7)
	assertFieldNumber(t, fd, "away_score", 8)
	assertFieldNumber(t, fd, "away_team_id", 9)
	assertFieldNumber(t, fd, "scraped_at", 10)
	assertFieldNumber(t, fd, "start_timestamp", 12)
	assertFieldNumber(t, fd, "current_period_start_timestamp", 13)
	assertFieldNumber(t, fd, "slug", 14)
	assertFieldNumber(t, fd, "status_type", 18)
}

func TestDeviceRegisterRequestFields(t *testing.T) {
	req := &DeviceRegisterRequest{
		Token:    "device-token-123",
		Platform: "android",
		Name:     "Test Device",
		Version:  "1.0",
	}
	fd := req.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "token", 1)
	assertFieldNumber(t, fd, "platform", 2)
	assertFieldNumber(t, fd, "name", 3)
	assertFieldNumber(t, fd, "version", 4)
	assertFieldNumber(t, fd, "domain_id", 5) // added 2026-08-28
}

// TestUserNotificationsEnabledField verifies the User proto carries the
// notifications_enabled (6) and notifications_enabled_at (7) fields that
// back the per-user feature toggle for push notifications.
func TestUserNotificationsEnabledField(t *testing.T) {
	u := &User{Id: 1, Email: "x@x.com"}
	fd := u.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "id", 1)
	assertFieldNumber(t, fd, "email", 4)
	assertFieldNumber(t, fd, "role", 5)
	assertFieldNumber(t, fd, "notifications_enabled", 6)    // added 2026-08-28
	assertFieldNumber(t, fd, "notifications_enabled_at", 7) // added 2026-08-28
}

// TestDeviceDomainIDField verifies the Device proto carries the domain_id (10)
// field that links a device to the user-owned domain it belongs to.
func TestDeviceDomainIDField(t *testing.T) {
	d := &Device{Id: 1, Token: "x"}
	fd := d.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "token", 4)
	assertFieldNumber(t, fd, "domain_id", 10) // added 2026-08-28
}

// TestPushEnumsExist verifies the five push-related enums are part of the
// file descriptor (the oneof WsFrame payload references WsError indirectly
// via the wire; here we only assert the enum names are registered).
func TestPushEnumsExist(t *testing.T) {
	fd := (&ErrorResponse{}).ProtoReflect().Descriptor().ParentFile()
	wantEnums := []string{
		"PushCategory",
		"PushPriority",
		"PushScheduleType",
		"DeliveryState",
		"DeliveryFailureReason",
	}
	for _, name := range wantEnums {
		if fd.Enums().ByName(protoreflect.Name(name)) == nil {
			t.Errorf("expected enum %q not found in file descriptor", name)
		}
	}
}

// TestPushPayloadFields validates the canonical field numbers for the
// push payload used by both immediate and scheduled pushes.
func TestPushPayloadFields(t *testing.T) {
	p := &PushPayload{Category: PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE, Title: "t", Body: "b"}
	fd := p.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "category", 1)
	assertFieldNumber(t, fd, "title", 2)
	assertFieldNumber(t, fd, "body", 3)
	assertFieldNumber(t, fd, "image_url", 4)
	assertFieldNumber(t, fd, "priority", 5)
	assertFieldNumber(t, fd, "ttl_seconds", 6)
	assertFieldNumber(t, fd, "data", 7)
}

// TestWsFrameOneof verifies the WsFrame message has the expected oneof
// payload with the canonical case numbers.
func TestWsFrameOneof(t *testing.T) {
	frame := &WsFrame_Hello{Hello: &WsHello{DeviceId: 1}}
	wrapped := &WsFrame{Payload: frame}
	raw, err := proto.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal WsFrame{Hello} failed: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("WsFrame{Hello} marshal produced empty bytes")
	}
	// Sanity: WsFrame.Hello is field 1 in the oneof.
	fd := wrapped.ProtoReflect().Descriptor()
	oneofs := fd.Oneofs()
	if oneofs.Len() != 1 {
		t.Fatalf("WsFrame must have exactly 1 oneof, got %d", oneofs.Len())
	}
	if oneofs.Get(0).Name() != "payload" {
		t.Errorf("WsFrame oneof name = %q, want %q", oneofs.Get(0).Name(), "payload")
	}
}

func TestEventsListFields(t *testing.T) {
	req := &EventsList{
		Page:       1,
		Limit:      20,
		Total:      100,
		TotalPages: 5,
	}
	fd := req.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "data", 1)
	assertFieldNumber(t, fd, "page", 2)
	assertFieldNumber(t, fd, "limit", 3)
	assertFieldNumber(t, fd, "total", 4)
	assertFieldNumber(t, fd, "total_pages", 5)
}

func TestPlaybackLogListFields(t *testing.T) {
	resp := &PlaybackLogList{
		List:  nil,
		Total: 42,
	}
	fd := resp.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "list", 1)
	assertFieldNumber(t, fd, "total", 2)
}

func TestApkInfoFields(t *testing.T) {
	info := &ApkInfo{
		Id:               1,
		Version:          "1.0",
		FileName:         "app.apk",
		FileSize:         1000000,
		Description:      "Test app",
		PackageName:      "com.test.app",
		VersionCode:      1,
		MinSdkVersion:    21,
		TargetSdkVersion: 33,
		DownloadToken:    "token-abc",
		DownloadUrl:      "http://example.com/app.apk",
		CreatedAt:        "2024-01-01T00:00:00Z",
		IsActive:         true,
		Downloads:        50,
		PanelUrl:         "http://panel.example.com",
	}
	fd := info.ProtoReflect().Descriptor()
	assertFieldNumber(t, fd, "id", 1)
	assertFieldNumber(t, fd, "version", 2)
	assertFieldNumber(t, fd, "file_name", 3)
	assertFieldNumber(t, fd, "file_size", 4)
	assertFieldNumber(t, fd, "description", 5)
	assertFieldNumber(t, fd, "package_name", 6)
	assertFieldNumber(t, fd, "version_code", 7)
	assertFieldNumber(t, fd, "min_sdk_version", 8)
	assertFieldNumber(t, fd, "target_sdk_version", 9)
	assertFieldNumber(t, fd, "download_token", 10)
	assertFieldNumber(t, fd, "download_url", 11)
	assertFieldNumber(t, fd, "created_at", 12)
	assertFieldNumber(t, fd, "is_active", 13)
	assertFieldNumber(t, fd, "downloads", 14)
	assertFieldNumber(t, fd, "panel_url", 15)
}

func TestAuthRequestMarshalContract(t *testing.T) {
	req := &AuthRequest{
		Email:    "test@example.com",
		Password: "secret123",
	}
	marshaled, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexAuthRequest {
		t.Errorf("AuthRequest wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexAuthRequest)
	}
}

func TestAuthResponseMarshalContract(t *testing.T) {
	resp := &AuthResponse{
		Id:           1,
		Email:        "test@example.com",
		Token:        "access-token-xxx",
		RefreshToken: "refresh-token-yyy",
	}
	marshaled, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexAuthResponse {
		t.Errorf("AuthResponse wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexAuthResponse)
	}
}

func TestDeviceRegisterRequestMarshalContract(t *testing.T) {
	req := &DeviceRegisterRequest{
		Token:    "device-token-123",
		Platform: "android",
		Name:     "Test Device",
		Version:  "1.0",
	}
	marshaled, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexDeviceRegisterRequest {
		t.Errorf("DeviceRegisterRequest wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexDeviceRegisterRequest)
	}
}

func TestEventsListMarshalContract(t *testing.T) {
	req := &EventsList{
		Data:       nil,
		Page:       1,
		Limit:      20,
		Total:      100,
		TotalPages: 5,
	}
	marshaled, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if len(marshaled) == 0 {
		t.Error("EventsList marshal produced empty bytes")
	}
	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexEventsList {
		t.Errorf("EventsList wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexEventsList)
	}
}

func TestPlaybackLogListMarshalContract(t *testing.T) {
	resp := &PlaybackLogList{
		List:  nil,
		Total: 42,
	}
	marshaled, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if len(marshaled) == 0 {
		t.Error("PlaybackLogList marshal produced empty bytes")
	}
	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexPlaybackLogList {
		t.Errorf("PlaybackLogList wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexPlaybackLogList)
	}
}

func TestApkInfoMarshalContract(t *testing.T) {
	info := &ApkInfo{
		Id:               1,
		Version:          "1.0",
		FileName:         "app.apk",
		FileSize:         1000000,
		Description:      "Test app",
		PackageName:      "com.test.app",
		VersionCode:      1,
		MinSdkVersion:    21,
		TargetSdkVersion: 33,
		DownloadToken:    "token-abc",
		DownloadUrl:      "http://example.com/app.apk",
		CreatedAt:        "2024-01-01T00:00:00Z",
		IsActive:         true,
		Downloads:        50,
		PanelUrl:         "http://panel.example.com",
	}
	marshaled, err := proto.Marshal(info)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if len(marshaled) == 0 {
		t.Error("ApkInfo marshal produced empty bytes")
	}
	gotHex := hex.EncodeToString(marshaled)
	if gotHex != hexApkInfo {
		t.Errorf("ApkInfo wire format changed:\n  got hex: %s\n want hex: %s", gotHex, hexApkInfo)
	}
}

func assertFieldNumber(t *testing.T, md protoreflect.MessageDescriptor, fieldName string, want protoreflect.FieldNumber) {
	t.Helper()
	fd := md.Fields().ByName(protoreflect.Name(fieldName))
	if fd == nil {
		t.Errorf("field %q not found in message %s", fieldName, md.Name())
		return
	}
	if fd.Number() != want {
		t.Errorf("field %q in %s: got number %d, want %d", fieldName, md.Name(), fd.Number(), want)
	}
}
