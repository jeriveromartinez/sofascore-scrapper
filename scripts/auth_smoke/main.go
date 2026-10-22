package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

const (
	envBaseURL        = "SMOKE_BASE_URL"
	envInvitation     = "SMOKE_INVITATION_TOKEN"
	envPassword       = "SMOKE_PASSWORD"
	defaultBaseURL    = "http://localhost:8080"
	passwordAlphabet  = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%"
	passwordMinLength = 20
)

func main() {
	base := os.Getenv(envBaseURL)
	if base == "" {
		base = defaultBaseURL
	}

	invitation := strings.TrimSpace(os.Getenv(envInvitation))
	if invitation == "" {
		fmt.Fprintf(os.Stderr, "%s is required: create an invitation from an admin account and export it\n", envInvitation)
		os.Exit(2)
	}

	email := fmt.Sprintf("admin-%d@test.local", time.Now().Unix())
	password := os.Getenv(envPassword)
	if password == "" {
		password = randomPassword()
	}

	if len(os.Args) > 1 {
		email = os.Args[1]
	}

	// 1. Register
	regReq := &pb.AuthRequest{
		Email:           email,
		Password:        password,
		InvitationToken: invitation,
	}
	regBody, _ := proto.Marshal(regReq)
	regResp, err := postProto(context.Background(), base+"/api/web/v1/users/register", regBody)
	if err != nil {
		fmt.Println("REGISTER ERR:", err)
		return
	}
	fmt.Println("REGISTER status:", regResp.status)

	if regResp.status != 201 {
		return
	}

	authResp := &pb.AuthResponse{}
	_ = proto.Unmarshal(regResp.body, authResp)
	fmt.Println("REGISTER ok: id=", authResp.Id, "email=", authResp.Email, "access_token=", preview(authResp.Token), "refresh_token=", preview(authResp.RefreshToken))

	// 2. Authenticated call: GET /api/web/v1/users
	req, _ := http.NewRequest("GET", base+"/api/web/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+authResp.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("USERS ERR:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	userList := &pb.UserList{}
	if err := proto.Unmarshal(body, userList); err != nil {
		fmt.Println("GET /api/web/v1/users status:", resp.StatusCode, "decode error:", err)
		return
	}
	fmt.Println("GET /api/web/v1/users status:", resp.StatusCode, "users:", len(userList.Users))

	// 3. Create invitation as admin
	invReq := &pb.CreateInvitationRequest{TtlSeconds: 3600}
	invBody, _ := proto.Marshal(invReq)
	invHTTP, err := postProtoWithAuth(context.Background(), base+"/api/web/v1/users/invitations", invBody, authResp.Token)
	if err != nil {
		fmt.Println("CREATE_INVITATION ERR:", err)
		return
	}
	fmt.Println("CREATE_INVITATION status:", invHTTP.status)

	invResp := &pb.InvitationResponse{}
	_ = proto.Unmarshal(invHTTP.body, invResp)
	fmt.Println("CREATE_INVITATION ok: token=", preview(invResp.Token), "expires_at=", invResp.ExpiresAt)

	// 4. Refresh token
	refReq, _ := http.NewRequest("POST", base+"/api/web/v1/users/refresh", nil)
	refReq.Header.Set("Authorization", "Bearer "+authResp.RefreshToken)
	refResp, err := http.DefaultClient.Do(refReq)
	if err != nil {
		fmt.Println("REFRESH ERR:", err)
		return
	}
	defer refResp.Body.Close()
	refBody, _ := io.ReadAll(refResp.Body)
	refreshed := &pb.AuthResponse{}
	if err := proto.Unmarshal(refBody, refreshed); err != nil {
		fmt.Println("REFRESH status:", refResp.StatusCode, "decode error:", err)
		return
	}
	fmt.Println("REFRESH status:", refResp.StatusCode, "access_token=", preview(refreshed.Token), "refresh_token=", preview(refreshed.RefreshToken))

	// 5. Logout
	logoutReq, _ := http.NewRequest("POST", base+"/api/web/v1/users/logout?all=true", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+authResp.Token)
	logoutResp, err := http.DefaultClient.Do(logoutReq)
	if err != nil {
		fmt.Println("LOGOUT ERR:", err)
		return
	}
	defer logoutResp.Body.Close()
	logoutBody, _ := io.ReadAll(logoutResp.Body)
	fmt.Println("LOGOUT status:", logoutResp.StatusCode, "body_len:", len(logoutBody))
}

type httpResp struct {
	status int
	body   []byte
}

// randomPassword builds a throwaway password that satisfies the account
// policy. The smoke account is disposable, so a random value keeps the
// credential out of the repository.
func randomPassword() string {
	out := make([]byte, passwordMinLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			out[i] = passwordAlphabet[i%len(passwordAlphabet)]
			continue
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out)
}

// preview renders a credential as its length plus a short digest, so an
// operator can tell two tokens apart without the secret reaching the
// terminal scrollback or a CI log.
func preview(token string) string {
	if token == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("<redacted len=%d sha256=%x>", len(token), sum[:4])
}

func postProto(ctx context.Context, url string, body []byte) (*httpResp, error) {
	return postProtoWithAuth(ctx, url, body, "")
}

func postProtoWithAuth(ctx context.Context, url string, body []byte, token string) (*httpResp, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return &httpResp{status: resp.StatusCode, body: b}, nil
}
