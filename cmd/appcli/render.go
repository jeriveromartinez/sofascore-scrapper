package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RenderEvents writes the events as a table. The clock is rendered in
// the given location so the simulator can show the times the device
// itself would display.
func RenderEvents(w io.Writer, list *pb.EventsList, loc *time.Location) error {
	if loc == nil {
		loc = time.Local
	}
	if len(list.GetData()) == 0 {
		_, err := fmt.Fprintln(w, "sin eventos para este device (revisa los torneos asignados a su dominio)")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "HORA\tLIGA\tLOCAL\tVISITANTE\tSCORE\tESTADO\tDEPORTE"); err != nil {
		return err
	}
	for _, ev := range list.GetData() {
		home := ""
		if ev.GetTeamHome() != nil {
			home = ev.GetTeamHome().GetName()
		}
		away := ""
		if ev.GetTeamAway() != nil {
			away = ev.GetTeamAway().GetName()
		}
		league := ""
		if ev.GetLeague() != nil {
			league = ev.GetLeague().GetName()
		}
		score := "-"
		if ev.GetHomeScore() != 0 || ev.GetAwayScore() != 0 {
			score = fmt.Sprintf("%d - %d", ev.GetHomeScore(), ev.GetAwayScore())
		}
		start := "-"
		if ev.GetStartTimestamp() != 0 {
			start = time.Unix(ev.GetStartTimestamp(), 0).In(loc).Format("15:04")
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			start, league, home, away, score, ev.GetStatusType(), ev.GetSport()); err != nil {
			return err
		}
	}
	return tw.Flush()
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
