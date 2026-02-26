package httpcli

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

func LoadData(sport string, date time.Time) []byte {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	_, err := client.Get("https://www.sofascore.com/es/")
	if err != nil {
		panic(err)
	}

	now := date.Format("2006-01-02")
	resp, err := client.Get("https://www.sofascore.com/api/v1/sport/" + sport + "/scheduled-events/" + now)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	return body
}
