package users

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/pagination"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const strongPassword = "correct-horse-battery"

func decodeUser(t *testing.T, w *httptest.ResponseRecorder) *pb.User {
	t.Helper()
	user := &pb.User{}
	if err := proto.Unmarshal(w.Body.Bytes(), user); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	return user
}

func seedPlainUsers(t *testing.T, db *gorm.DB, emails ...string) []User {
	t.Helper()
	seeded := make([]User, 0, len(emails))
	for _, email := range emails {
		u := User{Email: email, Password: "x"}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		seeded = append(seeded, u)
	}
	return seeded
}

func TestHandleCreateValidatesInput(t *testing.T) {
	r := newUsersTestRouter(newUsersTestDB(t), 1)

	cases := []struct {
		name     string
		req      *pb.UserWriteRequest
		wantBody string
	}{
		{"missing email", &pb.UserWriteRequest{Password: strongPassword}, `{"error":"email and password are required"}`},
		{"missing password", &pb.UserWriteRequest{Email: "a@test.local"}, `{"error":"email and password are required"}`},
		{"short password", &pb.UserWriteRequest{Email: "a@test.local", Password: "  short  "}, `{"error":"password must be at least 12 characters (got 5)"}`},
		{"common password", &pb.UserWriteRequest{Email: "a@test.local", Password: "Password1234"}, `{"error":"password is too common; choose a different one"}`},
	}
	for _, tc := range cases {
		w := sendProto(t, r, http.MethodPost, "/users", tc.req)
		if w.Code != http.StatusBadRequest || w.Body.String() != tc.wantBody {
			t.Errorf("%s: got %d %s, want 400 %s", tc.name, w.Code, w.Body.String(), tc.wantBody)
		}
	}
}

func TestHandleCreateStoresHashedUserAndRejectsDuplicate(t *testing.T) {
	db := newUsersTestDB(t)
	r := newUsersTestRouter(db, 1)
	req := &pb.UserWriteRequest{Email: "  new@test.local ", Password: strongPassword}

	w := sendProto(t, r, http.MethodPost, "/users", req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s, want 201", w.Code, w.Body.String())
	}
	created := decodeUser(t, w)
	if created.Email != "new@test.local" || created.Role != RoleUser {
		t.Fatalf("created user = %q/%q, want new@test.local/user", created.Email, created.Role)
	}
	var stored User
	if err := db.First(&stored, created.Id).Error; err != nil {
		t.Fatalf("load stored user: %v", err)
	}
	if stored.Password == strongPassword {
		t.Fatal("password stored in plain text")
	}

	if w := sendProto(t, r, http.MethodPost, "/users", req); w.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d %s, want 409", w.Code, w.Body.String())
	}
}

func TestHandleUpdate(t *testing.T) {
	db := newUsersTestDB(t)
	r := newUsersTestRouter(db, 1)
	target := seedPlainUsers(t, db, "old@test.local", "taken@test.local")[0]

	badRequests := []struct {
		name, url string
		req       *pb.UserWriteRequest
		wantBody  string
	}{
		{"invalid id", "/users/abc", &pb.UserWriteRequest{Email: "x@test.local"}, `{"error":"invalid id"}`},
		{"missing email", "/users/1", &pb.UserWriteRequest{Password: strongPassword}, `{"error":"email is required"}`},
		{"short password", "/users/1", &pb.UserWriteRequest{Email: "x@test.local", Password: "short"}, `{"error":"password must be at least 12 characters (got 5)"}`},
		{"common password", "/users/1", &pb.UserWriteRequest{Email: "x@test.local", Password: "qwerty123456"}, `{"error":"password is too common; choose a different one"}`},
	}
	for _, tc := range badRequests {
		w := sendProto(t, r, http.MethodPut, tc.url, tc.req)
		if w.Code != http.StatusBadRequest || w.Body.String() != tc.wantBody {
			t.Errorf("%s: got %d %s, want 400 %s", tc.name, w.Code, w.Body.String(), tc.wantBody)
		}
	}

	if w := sendProto(t, r, http.MethodPut, "/users/999", &pb.UserWriteRequest{Email: "x@test.local"}); w.Code != http.StatusNotFound {
		t.Errorf("unknown user = %d %s, want 404", w.Code, w.Body.String())
	}
	if w := sendProto(t, r, http.MethodPut, "/users/1", &pb.UserWriteRequest{Email: "taken@test.local"}); w.Code != http.StatusConflict {
		t.Errorf("email collision = %d %s, want 409", w.Code, w.Body.String())
	}

	w := sendProto(t, r, http.MethodPut, "/users/1", &pb.UserWriteRequest{Email: "renamed@test.local"})
	if w.Code != http.StatusOK || decodeUser(t, w).Email != "renamed@test.local" {
		t.Fatalf("email-only update = %d %s, want 200 with new email", w.Code, w.Body.String())
	}
	var stored User
	if err := db.First(&stored, target.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if stored.Password != target.Password {
		t.Fatal("email-only update changed the password hash")
	}

	if w := sendProto(t, r, http.MethodPut, "/users/1", &pb.UserWriteRequest{Email: "renamed@test.local", Password: strongPassword}); w.Code != http.StatusOK {
		t.Fatalf("password update = %d %s, want 200", w.Code, w.Body.String())
	}
	if err := db.First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Password == target.Password || stored.Password == strongPassword {
		t.Fatal("password update did not store a new hash")
	}
}

func TestHandleDelete(t *testing.T) {
	db := newUsersTestDB(t)
	for _, ddl := range []string{
		"CREATE TABLE devices (id INTEGER PRIMARY KEY, user_id INTEGER)",
		"CREATE TABLE domains (id INTEGER PRIMARY KEY, user_id INTEGER)",
		"CREATE TABLE refresh_tokens (id INTEGER PRIMARY KEY, user_id INTEGER)",
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create dependent table: %v", err)
		}
	}
	r := newUsersTestRouter(db, 1)
	target := seedPlainUsers(t, db, "gone@test.local")[0]
	for _, insert := range []string{
		"INSERT INTO devices (user_id) VALUES (?)",
		"INSERT INTO domains (user_id) VALUES (?)",
		"INSERT INTO refresh_tokens (user_id) VALUES (?)",
	} {
		if err := db.Exec(insert, target.ID).Error; err != nil {
			t.Fatalf("seed dependent row: %v", err)
		}
	}

	if w := sendProto(t, r, http.MethodDelete, "/users/abc", nil); w.Code != http.StatusBadRequest {
		t.Errorf("invalid id = %d, want 400", w.Code)
	}
	if w := sendProto(t, r, http.MethodDelete, "/users/999", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown user = %d %s, want 404", w.Code, w.Body.String())
	}
	if w := sendProto(t, r, http.MethodDelete, "/users/1", nil); w.Code != http.StatusOK {
		t.Fatalf("delete = %d %s, want 200", w.Code, w.Body.String())
	}

	counts := map[string]string{
		"live users":     "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL",
		"owned devices":  "SELECT COUNT(*) FROM devices WHERE user_id IS NOT NULL",
		"domains":        "SELECT COUNT(*) FROM domains",
		"refresh tokens": "SELECT COUNT(*) FROM refresh_tokens",
	}
	for name, query := range counts {
		var n int64
		if err := db.Raw(query).Scan(&n).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if n != 0 {
			t.Errorf("after delete: %s = %d, want 0", name, n)
		}
	}
}

func TestHandleGetUsersListsByEmail(t *testing.T) {
	db := newUsersTestDB(t)
	seedPlainUsers(t, db, "c@test.local", "a@test.local", "b@test.local")
	r := newUsersTestRouter(db, 1)

	w := sendProto(t, r, http.MethodGet, "/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d %s, want 200", w.Code, w.Body.String())
	}
	list := &pb.UserList{}
	if err := proto.Unmarshal(w.Body.Bytes(), list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var got []string
	for _, u := range list.Users {
		got = append(got, u.Email)
	}
	if fmt.Sprint(got) != "[a@test.local b@test.local c@test.local]" {
		t.Fatalf("emails = %v, want sorted a,b,c", got)
	}
}

func getUsersPage(t *testing.T, r *gin.Engine, target string) *pb.UserPage {
	t.Helper()
	w := sendProto(t, r, http.MethodGet, target, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s, want 200", target, w.Code, w.Body.String())
	}
	page := &pb.UserPage{}
	if err := proto.Unmarshal(w.Body.Bytes(), page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return page
}

func TestHandleGetUsersPageWalksCursor(t *testing.T) {
	db := newUsersTestDB(t)
	seedPlainUsers(t, db, "e@test.local", "a@test.local", "d@test.local", "b@test.local", "c@test.local")
	r := newUsersTestRouter(db, 1)

	var walked []string
	target := "/users/page?limit=2"
	for pages := 0; ; pages++ {
		if pages > 3 {
			t.Fatal("cursor never reported the last page")
		}
		page := getUsersPage(t, r, target)
		for _, u := range page.Data {
			walked = append(walked, u.Email)
		}
		if !page.Page.GetHasMore() {
			if page.Page.GetNextCursor() != "" {
				t.Fatalf("last page carries next cursor %q", page.Page.GetNextCursor())
			}
			break
		}
		target = "/users/page?limit=2&cursor=" + url.QueryEscape(page.Page.GetNextCursor())
	}
	if fmt.Sprint(walked) != "[a@test.local b@test.local c@test.local d@test.local e@test.local]" {
		t.Fatalf("walked %v, want a..e once each", walked)
	}
}

func TestHandleGetUsersPageLimitAndCursorValidation(t *testing.T) {
	db := newUsersTestDB(t)
	emails := make([]string, 0, defaultPageLimit+5)
	for i := 0; i < defaultPageLimit+5; i++ {
		emails = append(emails, fmt.Sprintf("u%02d@test.local", i))
	}
	seedPlainUsers(t, db, emails...)
	r := newUsersTestRouter(db, 1)

	for _, limit := range []string{"0", "-1", "101", "abc"} {
		if got := len(getUsersPage(t, r, "/users/page?limit="+limit).Data); got != defaultPageLimit {
			t.Errorf("limit=%s returned %d rows, want default %d", limit, got, defaultPageLimit)
		}
	}
	if got := len(getUsersPage(t, r, "/users/page?limit=100").Data); got != len(emails) {
		t.Errorf("limit=100 returned %d rows, want %d", got, len(emails))
	}

	badID, err := pagination.Encode("u00@test.local", "not-a-number")
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	wrongArity, err := pagination.Encode("u00@test.local")
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	for cursor, want := range map[string]string{
		"not base64!": `{"error":"invalid cursor"}`,
		wrongArity:    `{"error":"invalid cursor"}`,
		badID:         `{"error":"invalid cursor: bad id"}`,
	} {
		w := sendProto(t, r, http.MethodGet, "/users/page?cursor="+url.QueryEscape(cursor), nil)
		if w.Code != http.StatusBadRequest || w.Body.String() != want {
			t.Errorf("cursor %q = %d %s, want 400 %s", cursor, w.Code, w.Body.String(), want)
		}
	}
}
