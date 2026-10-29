package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

const (
	// DefaultBaseURL points at the local dev stack (compose.dev.yml
	// maps the API to 8080).
	DefaultBaseURL = "http://localhost:8080"
	// DefaultPackage is the Android package name the real TV app
	// registers with (flutter-tv/android/app/build.gradle).
	DefaultPackage = "mx.qchotv.tvapp"
	// DefaultAppVersion is the version the real TV app publishes
	// (flutter-tv/pubspec.yaml).
	DefaultAppVersion = "1.0.22"
	// DefaultAppName is the app name the crash report carries.
	DefaultAppName = "tvapp"
	// DefaultPlatform is the platform the Android build reports.
	DefaultPlatform = "android"
	// DefaultDeviceName is the name the simulator registers with.
	DefaultDeviceName = "appcli-sim"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runWithContext(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// options holds every flag the commands share.
type options struct {
	baseURL    string
	token      string
	pkg        string
	platform   string
	version    string
	name       string
	timezone   string
	limit      int
	jsonOut    bool
	duration   time.Duration
	noAck      bool
	build      string
	device     string
	locale     string
	contentNm  string
	contentURL string
	errMsg     string
	stack      string
	context    string
	fatal      bool
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithContext(context.Background(), args, stdout, stderr)
}

func runWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	command := args[0]
	opts, err := parseOptions(command, args[1:], stderr)
	if err != nil {
		return 2
	}

	switch command {
	case "register":
		return opts.cmdRegister(ctx, stdout, stderr)
	case "update":
		return opts.cmdUpdate(ctx, stdout, stderr)
	case "events":
		return opts.cmdEvents(ctx, stdout, stderr)
	case "viewing":
		return opts.cmdViewing(ctx, stdout, stderr)
	case "crash":
		return opts.cmdCrash(ctx, stdout, stderr)
	case "ws":
		return opts.cmdWS(ctx, stdout, stderr)
	case "session":
		return opts.cmdSession(ctx, stdout, stderr)
	case "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "appcli: unknown command %q\n\n", command)
		printUsage(stderr)
		return 2
	}
}

func parseOptions(command string, args []string, stderr io.Writer) (*options, error) {
	opts := &options{}
	fs := flag.NewFlagSet("appcli "+command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "appcli %s [flags]\n\nFlags:\n", command)
		fs.PrintDefaults()
	}

	fs.StringVar(&opts.baseURL, "base-url", envOr("APPCLI_BASE_URL", DefaultBaseURL), "base URL of the API")
	fs.StringVar(&opts.token, "token", os.Getenv("APP_XIPTV_TOKEN"), "device token sent as APP-XIPTV")
	fs.StringVar(&opts.pkg, "package", DefaultPackage, "application package name")
	fs.StringVar(&opts.platform, "platform", DefaultPlatform, "device platform")
	fs.StringVar(&opts.version, "version", DefaultAppVersion, "app version")
	fs.StringVar(&opts.name, "name", DefaultDeviceName, "device name")
	fs.StringVar(&opts.timezone, "timezone", os.Getenv("TZ"), "IANA timezone of the device")
	fs.IntVar(&opts.limit, "limit", 6, "number of events to request (1-6)")
	fs.BoolVar(&opts.jsonOut, "json", false, "print the raw decoded payload as JSON")
	fs.DurationVar(&opts.duration, "duration", 0, "stop listening after this duration (0 = until interrupted)")
	fs.BoolVar(&opts.noAck, "no-ack", false, "do not acknowledge the pushes received")
	fs.StringVar(&opts.build, "build", "22", "build number used in the crash report")
	fs.StringVar(&opts.device, "os-version", "Android 14", "OS version used in the crash report")
	fs.StringVar(&opts.locale, "locale", "es_MX", "locale used in the crash report")
	fs.StringVar(&opts.contentNm, "content-name", "Canal 1", "name of the content that started playing")
	fs.StringVar(&opts.contentURL, "content-url", "https://panel.example.com/stream.m3u8", "URL of the content that started playing")
	fs.StringVar(&opts.errMsg, "error", "Null check operator used on a null value", "error message of the crash report")
	fs.StringVar(&opts.stack, "stack", "#0 appcli (simulated crash)", "stack trace of the crash report")
	fs.StringVar(&opts.context, "context", "channel player", "context of the crash report")
	fs.BoolVar(&opts.fatal, "fatal", false, "mark the crash report as fatal")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if opts.timezone == "" {
		opts.timezone = time.Now().Location().String()
	}
	return opts, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `appcli - simulador de cliente para la API /api/app/v1

Uso:
  appcli <comando> [flags]

Comandos:
  register   registra el device (POST /devices) e imprime su token
  update     consulta si hay una version nueva de la app (GET /update)
  events     lista los eventos actuales del device (GET /current-events)
  viewing    reporta que el device empezo a reproducir (POST /devices/viewing)
  crash      envia un crash report (POST /crash-report)
  ws         escucha el WebSocket e imprime cada notificacion recibida
  session    ejecuta el flujo completo: register, update, events y ws
  help       muestra esta ayuda

Ejemplo:
  appcli session --base-url http://localhost:8080 --duration 60s
`)
}

func (o *options) client(token string) *Client {
	return NewClient(o.baseURL, token)
}

// location returns the timezone the events are rendered in.
func (o *options) location() *time.Location {
	if o.timezone != "" {
		if loc, err := time.LoadLocation(o.timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

func (o *options) cmdRegister(ctx context.Context, stdout, stderr io.Writer) int {
	token := o.token
	if token == "" {
		token = uuid.NewString()
	}

	dev, err := o.client(token).Register(ctx, &pb.DeviceRegisterRequest{
		Token:     token,
		Platform:  o.platform,
		Name:      o.name,
		Version:   o.version,
		PackageId: o.pkg,
		Timezone:  o.timezone,
	})
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}

	if o.jsonOut {
		return printJSON(stdout, stderr, dev)
	}

	fmt.Fprintf(stdout, "device #%d registrado\n", dev.Id)
	fmt.Fprintf(stdout, "  token:       %s\n", deviceToken(dev, token))
	fmt.Fprintf(stdout, "  platform:    %s\n", dev.Platform)
	fmt.Fprintf(stdout, "  name:        %s\n", dev.Name)
	fmt.Fprintf(stdout, "  version:     %s\n", dev.Version)
	fmt.Fprintf(stdout, "  package:     %s\n", dev.PackageId)
	fmt.Fprintf(stdout, "  domain:      %d\n", dev.DomainId)
	fmt.Fprintf(stdout, "  iptv url:    %s\n", dev.IptvUrl)
	return 0
}

func (o *options) cmdUpdate(ctx context.Context, stdout, stderr io.Writer) int {
	resp, err := o.client(o.token).CheckUpdate(ctx, o.version, o.pkg)
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}

	if o.jsonOut {
		return printJSON(stdout, stderr, resp)
	}

	if resp.UpdateAvailable {
		fmt.Fprintf(stdout, "hay actualizacion: %s (codigo %d)\n", resp.LatestVersion, resp.VersionCode)
		if resp.DownloadUrl != "" {
			fmt.Fprintf(stdout, "  descarga: %s\n", o.baseURL+resp.DownloadUrl)
		}
		if resp.Description != "" {
			fmt.Fprintf(stdout, "  detalle:  %s\n", resp.Description)
		}
		return 0
	}
	fmt.Fprintf(stdout, "no hay actualizacion (version actual %s)\n", o.version)
	return 0
}

func (o *options) cmdEvents(ctx context.Context, stdout, stderr io.Writer) int {
	list, err := o.client(o.token).CurrentEvents(ctx, o.limit)
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}

	if o.jsonOut {
		return printJSON(stdout, stderr, list)
	}
	if err := RenderEvents(stdout, list, o.location()); err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	return 0
}

func (o *options) cmdViewing(ctx context.Context, stdout, stderr io.Writer) int {
	// The app hashes the content as base64("name~url").
	content := base64.StdEncoding.EncodeToString([]byte(o.contentNm + "~" + o.contentURL))
	err := o.client(o.token).ReportViewing(ctx, &pb.LogPlaybackRequest{
		DeviceToken: o.token,
		Content:     content,
		StartedAt:   time.Now().UnixMilli(),
	})
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	fmt.Fprintln(stdout, "reproduccion reportada")
	return 0
}

func (o *options) cmdCrash(ctx context.Context, stdout, stderr io.Writer) int {
	err := o.client(o.token).ReportCrash(ctx, &CrashReport{
		Fatal:      o.fatal,
		Error:      o.errMsg,
		StackTrace: o.stack,
		Context:    o.context,
		App: AppReport{
			Name:        DefaultAppName,
			Version:     o.version,
			Build:       o.build,
			Environment: "debug",
			Platform:    o.platform,
		},
		Device: DeviceReport{
			OsVersion: o.device,
			Locale:    o.locale,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	fmt.Fprintln(stdout, "crash report enviado")
	return 0
}

func (o *options) cmdWS(ctx context.Context, stdout, stderr io.Writer) int {
	if ctx.Err() != nil {
		return 0
	}
	if o.token == "" {
		fmt.Fprintln(stderr, "appcli: falta el token del device (--token)")
		return 1
	}

	listenCtx := ctx
	if o.duration > 0 {
		var cancel context.CancelFunc
		listenCtx, cancel = context.WithTimeout(ctx, o.duration)
		defer cancel()
	}

	client := NewWSClient(o.baseURL, o.token)
	err := client.Listen(listenCtx, WSHandlers{
		NoAck: o.noAck,
		OnHello: func(h Hello) {
			fmt.Fprintf(stdout, "conectado como device #%d\n", h.DeviceID)
		},
		OnPush: func(p PushEvent) error {
			if o.jsonOut {
				raw, err := MarshalJSON(pushToProto(p))
				if err != nil {
					return err
				}
				fmt.Fprintln(stdout, string(raw))
				return nil
			}
			return RenderPush(stdout, p, !o.noAck)
		},
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	return 0
}

func (o *options) cmdSession(ctx context.Context, stdout, stderr io.Writer) int {
	token := o.token
	if token == "" {
		token = uuid.NewString()
	}

	fmt.Fprintln(stdout, "== registro ==")
	dev, err := o.client(token).Register(ctx, &pb.DeviceRegisterRequest{
		Token:     token,
		Platform:  o.platform,
		Name:      o.name,
		Version:   o.version,
		PackageId: o.pkg,
		Timezone:  o.timezone,
	})
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	if o.jsonOut {
		if code := printJSON(stdout, stderr, dev); code != 0 {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "device #%d (token %s) domain %d\n", dev.Id, deviceToken(dev, token), dev.DomainId)
		if dev.IptvUrl != "" {
			fmt.Fprintf(stdout, "iptv url: %s\n", dev.IptvUrl)
		}
	}

	fmt.Fprintln(stdout, "\n== actualizacion ==")
	o.token = token
	if code := o.cmdUpdate(ctx, stdout, stderr); code != 0 {
		return code
	}

	fmt.Fprintln(stdout, "\n== eventos ==")
	if code := o.cmdEvents(ctx, stdout, stderr); code != 0 {
		return code
	}

	fmt.Fprintln(stdout, "\n== notificaciones ==")
	return o.cmdWS(ctx, stdout, stderr)
}

// deviceToken returns the token the server echoed back, falling back
// to the one the simulator sent when the answer omits it.
func deviceToken(dev *pb.Device, sent string) string {
	if dev.GetToken() != "" {
		return dev.GetToken()
	}
	return sent
}

func pushToProto(p PushEvent) *pb.WsPush {
	return &pb.WsPush{
		PushId:     p.PushID,
		MessageId:  p.MessageID,
		Category:   p.Category,
		Title:      p.Title,
		Body:       p.Body,
		ImageUrl:   p.ImageURL,
		Priority:   p.Priority,
		TtlSeconds: p.TTLSeconds,
		Data:       p.Data,
		SentAt:     p.SentAt,
	}
}

func printJSON(w io.Writer, stderr io.Writer, msg proto.Message) int {
	raw, err := MarshalJSON(msg)
	if err != nil {
		fmt.Fprintln(stderr, "appcli:", err)
		return 1
	}
	fmt.Fprintln(w, strings.TrimRight(string(raw), "\n"))
	return 0
}
