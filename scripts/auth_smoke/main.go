package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

func main() {
	base := "http://localhost:8080"
	email := fmt.Sprintf("admin-%d@test.local", time.Now().Unix())
	password := "CorrectHorseBatteryStaple!9"
	invitation := "FUZ_TeA3rFVByKolpH0fbuHGCb8Zf9Vc3BUK4k3wpz8"

	if len(os.Args) > 1 {
		email = os.Args[1]
	}
	_ = email

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
	fmt.Println("REGISTER status:", regResp.status, "body:", regResp.body)

	if regResp.status != 201 {
		return
	}

	authResp := &pb.AuthResponse{}
	_ = proto.Unmarshal(regResp.body, authResp)
	fmt.Println("REGISTER ok: id=", authResp.Id, "email=", authResp.Email, "access_token=", authResp.Token[:32]+"...", "refresh_token=", authResp.RefreshToken[:32]+"...")

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
	fmt.Println("GET /api/web/v1/users status:", resp.StatusCode, "body[:200]:", string(body)[:min(200, len(body))])

	// 3. Create invitation as admin
	invReq := &pb.CreateInvitationRequest{TtlSeconds: 3600}
	invBody, _ := proto.Marshal(invReq)
	invHTTP, err := postProtoWithAuth(context.Background(), base+"/api/web/v1/users/invitations", invBody, authResp.Token)
	if err != nil {
		fmt.Println("CREATE_INVITATION ERR:", err)
		return
	}
	fmt.Println("CREATE_INVITATION status:", invHTTP.status, "body:", invHTTP.body)

	invResp := &pb.InvitationResponse{}
	_ = proto.Unmarshal(invHTTP.body, invResp)
	fmt.Println("CREATE_INVITATION ok: token=", invResp.Token, "expires_at=", invResp.ExpiresAt)

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
	fmt.Println("REFRESH status:", refResp.StatusCode, "body[:200]:", string(refBody)[:min(200, len(refBody))])

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
	fmt.Println("LOGOUT status:", logoutResp.StatusCode, "body:", strings.TrimSpace(string(logoutBody)))
}

type httpResp struct {
	status int
	body   []byte
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
