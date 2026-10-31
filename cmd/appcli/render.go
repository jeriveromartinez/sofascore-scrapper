package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// defaultTeamColor is the grey the dashboard uses when a team has no
// parseable primary colour.
const defaultTeamColor = "0xFF888888"

// RenderEvents writes the same fields the dashboard builds for its
// slider: local day and time, "home vs away", league, team colours and
// team logos. dashboardDomain is prepended to the logo paths the server
// returns, exactly like EventsApi.listNext.
func RenderEvents(w io.Writer, list *pb.EventsList, loc *time.Location, dashboardDomain string) error {
	if loc == nil {
		loc = time.Local
	}
	if len(list.GetData()) == 0 {
		_, err := fmt.Fprintln(w, "sin eventos para este device (revisa los torneos asignados a su dominio)")
		return err
	}

	now := time.Now().In(loc)
	for _, ev := range list.GetData() {
		home := ev.GetTeamHome()
		away := ev.GetTeamAway()

		day := "-"
		clock := "-"
		if ev.GetStartTimestamp() != 0 {
			start := time.UnixMilli(ev.GetStartTimestamp()).In(loc)
			clock = start.Format("15:04")
			if start.Day() == now.Day() && start.Month() == now.Month() && start.Year() == now.Year() {
				day = "hoy"
			} else {
				day = fmt.Sprintf("%d/%d", start.Day(), int(start.Month()))
			}
		}

		league := ""
		if ev.GetLeague() != nil {
			league = ev.GetLeague().GetName()
		}
		score := "-"
		if ev.GetHomeScore() != 0 || ev.GetAwayScore() != 0 {
			score = fmt.Sprintf("%d - %d", ev.GetHomeScore(), ev.GetAwayScore())
		}

		if _, err := fmt.Fprintf(w, "[%s %s] %s vs %s\n", day, clock, home.GetName(), away.GetName()); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  liga:      %s\n", league); err != nil {
			return err
		}
		if err := renderTeamLine(w, "local:    ", home, dashboardDomain); err != nil {
			return err
		}
		if err := renderTeamLine(w, "visitante:", away, dashboardDomain); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  estado:    %s  deporte: %s  score: %s\n\n",
			ev.GetStatusType(), ev.GetSport(), score); err != nil {
			return err
		}
	}
	return nil
}

func renderTeamLine(w io.Writer, label string, team *pb.Team, dashboardDomain string) error {
	_, err := fmt.Fprintf(w, "  %s %s  color: %s  logo: %s%s\n",
		label, team.GetName(), dashboardColor(team.GetPrimaryColor()), dashboardDomain, team.GetLogoUrl())
	return err
}

// dashboardColor mirrors EventsApi.listNext: "#rrggbb" becomes the
// 0xFFrrggbb integer the TV renders, anything unparseable is grey.
func dashboardColor(primaryColor string) string {
	raw := strings.Replace(primaryColor, "#", "ff", 1)
	value, err := strconv.ParseUint(raw, 16, 32)
	if err != nil {
		return defaultTeamColor
	}
	return fmt.Sprintf("0x%08X", value)
}

// RenderPush writes one received notification.
func RenderPush(w io.Writer, push PushEvent, acked bool) error {
	ackText := "ack enviado"
	if !acked {
		ackText = "sin ack"
	}
	_, err := fmt.Fprintf(w, "%s push #%d [%s] %s — %s (%s)\n",
		time.Now().Format("15:04:05"),
		push.PushID,
		push.Category,
		push.Title,
		push.Body,
		ackText)
	return err
}

// MarshalJSON renders a protobuf message as stable, indented JSON.
// protojson alone is not deterministic (it injects random spacing), so
// the payload is re-encoded through encoding/json, whose map keys are
// sorted.
func MarshalJSON(msg proto.Message) ([]byte, error) {
	raw, err := protojson.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(decoded); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}
