package httpcli

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
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

func loadCookies() *http.Client {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	homeReq, err := http.NewRequest(http.MethodGet, "https://www.sofascore.com/es/", nil)
	if err != nil {
		return nil
	}
	setBrowserHeaders(homeReq, "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8", "")

	homeResp, err := client.Do(homeReq)
	if err != nil {
		return nil
	}

	defer homeResp.Body.Close()
	if homeResp.StatusCode < 200 || homeResp.StatusCode >= 305 {
		return nil
	}
	_, _ = io.Copy(io.Discard, homeResp.Body)
	return client
}

func LoadDataBySport(sport string, date time.Time) []byte {
	client := loadCookies()
	if client == nil {
		return nil
	}

	now := date.Format("2006-01-02")
	apiURL := "https://www.sofascore.com/api/v1/sport/" + sport + "/scheduled-events/" + now
	apiReq, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}

	setBrowserHeaders(apiReq, "application/json, text/plain, */*", "https://www.sofascore.com/es/")
	resp, err := client.Do(apiReq)
	if err != nil {
		return nil
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 305 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return body
}

func LoadDataByTrendingCountry(countryCode string) []byte {
	client := loadCookies()
	if client == nil {
		return nil
	}

	apiURL := "https://www.sofascore.com/api/v1/trending/events/" + strings.ToUpper(countryCode) + "/all"
	apiReq, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}

	setBrowserHeaders(apiReq, "application/json, text/plain, */*", "https://www.sofascore.com/es/")
	resp, err := client.Do(apiReq)
	if err != nil {
		return nil
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 305 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return body
}
