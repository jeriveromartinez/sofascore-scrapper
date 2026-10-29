package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

// AppUserAgent is the User-Agent the Flutter clients send on every
// request (lib/shared/constants/app.dart in flutter-apptv and
// flutter-tv). Sending the same value keeps the simulator
// indistinguishable from the real app on the server side.
const AppUserAgent = "J3rmTV/0.0.0a (FL)"

// appPathPrefix is the versioned prefix of the app-facing API.
const appPathPrefix = "/api/app/v1"

// Client talks to the app-facing API the way the Flutter client does:
// protobuf bodies, the APP-XIPTV device token header and the app
// User-Agent.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient returns a client for the API rooted at baseURL. The token
// is the device token the app registers with; it is sent as the
// APP-XIPTV header on every request.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is returned when the server answers with a non-2xx status.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error %d: %s", e.Status, e.Message)
}

// Register creates (or refreshes) the device on the server and returns
// the stored row.
func (c *Client) Register(ctx context.Context, req *pb.DeviceRegisterRequest) (*pb.Device, error) {
	body, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode register request: %w", err)
	}
	raw, err := c.do(ctx, http.MethodPost, "/devices", nil, body)
	if err != nil {
		return nil, err
	}
	dev := &pb.Device{}
	if err := proto.Unmarshal(raw, dev); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}
	return dev, nil
}

// CurrentEvents returns the events the device is entitled to see.
func (c *Client) CurrentEvents(ctx context.Context, limit int) (*pb.EventsList, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	raw, err := c.do(ctx, http.MethodGet, "/current-events", query, nil)
	if err != nil {
		return nil, err
	}
	list := &pb.EventsList{}
	if err := proto.Unmarshal(raw, list); err != nil {
		return nil, fmt.Errorf("decode current events: %w", err)
	}
	return list, nil
}

// CheckUpdate asks whether a newer APK is published for the package.
func (c *Client) CheckUpdate(ctx context.Context, version, pkg string) (*pb.ApkUpdateCheckResponse, error) {
	query := url.Values{"version": {version}, "package": {pkg}}
	raw, err := c.do(ctx, http.MethodGet, "/update", query, nil)
	if err != nil {
		return nil, err
	}
	resp := &pb.ApkUpdateCheckResponse{}
	if err := proto.Unmarshal(raw, resp); err != nil {
		return nil, fmt.Errorf("decode update response: %w", err)
	}
	return resp, nil
}

// DeviceURL returns the IPTV panel URL published for the package.
func (c *Client) DeviceURL(ctx context.Context, pkg string) (string, error) {
	raw, err := c.do(ctx, http.MethodGet, "/devices/url/"+url.PathEscape(pkg), nil, nil)
	if err != nil {
		return "", err
	}
	devURL := &pb.DeviceUrl{}
	if err := proto.Unmarshal(raw, devURL); err != nil {
		return "", fmt.Errorf("decode device url: %w", err)
	}
	return devURL.Url, nil
}

// ReportViewing registers that the device started playing a stream.
func (c *Client) ReportViewing(ctx context.Context, req *pb.LogPlaybackRequest) error {
	body, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode playback request: %w", err)
	}
	if _, err := c.do(ctx, http.MethodPost, "/devices/viewing", nil, body); err != nil {
		return err
	}
	return nil
}

// ReportCrash uploads a crash report. The endpoint is public: an app
// may crash before it holds a session, so no device token is sent.
func (c *Client) ReportCrash(ctx context.Context, report *CrashReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode crash report: %w", err)
	}
	if _, err := c.doRaw(ctx, http.MethodPost, "/crash-report", nil, body, "application/json"); err != nil {
		return err
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte) ([]byte, error) {
	return c.doRaw(ctx, method, path, query, body, "application/x-protobuf")
}

func (c *Client) doRaw(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) ([]byte, error) {
	resp, raw, err := c.request(ctx, method, path, query, body, contentType)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return raw, nil
}

// request builds and performs the call, returning the response and
// its body already read.
func (c *Client) request(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) (*http.Response, []byte, error) {
	endpoint := c.baseURL + appPathPrefix + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", contentType)
	}
	req.Header.Set("User-Agent", AppUserAgent)
	if c.token != "" {
		req.Header.Set("APP-XIPTV", c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	return resp, raw, nil
}

// FileResponse is a binary answer: the bytes the server sent plus the
// metadata that describes them.
type FileResponse struct {
	ContentType string
	Filename    string
	Body        []byte
}

// GetFile fetches a binary resource: the APK of a package or the logo
// of a team.
func (c *Client) GetFile(ctx context.Context, path string) (*FileResponse, error) {
	resp, raw, err := c.request(ctx, http.MethodGet, path, nil, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return &FileResponse{
		ContentType: resp.Header.Get("Content-Type"),
		Filename:    filenameFromDisposition(resp.Header.Get("Content-Disposition")),
		Body:        raw,
	}, nil
}

// filenameFromDisposition extracts the file name the server suggests.
func filenameFromDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// errorMessage extracts the message the API puts in its JSON error
// body, falling back to the raw body when it is not JSON.
func errorMessage(raw []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	return strings.TrimSpace(string(raw))
}
