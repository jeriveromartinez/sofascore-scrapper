package httpcli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

func setBrowserHeaders(req *http.Request, accept string, referer string) {
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func LoadData(sport string, date time.Time) []byte {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second}

	homeReq, err := http.NewRequest(http.MethodGet, "https://www.sofascore.com/es/", nil)
	if err != nil {
		panic(err)
	}
	setBrowserHeaders(homeReq, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", "")

	homeResp, err := client.Do(homeReq)
	if err != nil {
		panic(err)
	}

	defer homeResp.Body.Close()
	if homeResp.StatusCode < 200 || homeResp.StatusCode >= 305 {
		panic(fmt.Errorf("sofascore home request failed with status %d", homeResp.StatusCode))
	}
	_, _ = io.Copy(io.Discard, homeResp.Body)

	now := date.Format("2006-01-02")
	apiURL := "https://www.sofascore.com/api/v1/sport/" + sport + "/scheduled-events/" + now
	apiReq, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		panic(err)
	}

	setBrowserHeaders(apiReq, "application/json, text/plain, */*", "https://www.sofascore.com/es/")
	resp, err := client.Do(apiReq)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 305 {
		panic(fmt.Errorf("sofascore api request failed with status %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	return body
}
