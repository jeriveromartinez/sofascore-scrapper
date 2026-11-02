package devices

import (
	"net/http"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

func TestRegisterDeviceAnswersThePanelURLOfThePackage(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "panel.example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "https://panel.example.com", true)
	router := newDevicesTestRouter(db)

	w := sendDeviceRequest(t, router, http.MethodPost, "/app/devices",
		&pb.DeviceRegisterRequest{Token: "tok", Name: "Box", PackageId: "com.example.app"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var device pb.Device
	if err := proto.Unmarshal(w.Body.Bytes(), &device); err != nil {
		t.Fatalf("decode device: %v", err)
	}
	if device.IptvUrl != "https://panel.example.com" {
		t.Fatalf("iptv_url = %q, want %q", device.IptvUrl, "https://panel.example.com")
	}
}

func TestRegisterDeviceLeavesThePanelURLEmptyWhenThePackageHasNoAPK(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "panel.example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "https://panel.example.com", true)
	router := newDevicesTestRouter(db)

	// The package of the request has no published APK, but the explicit
	// domain still lets the registration through.
	w := sendDeviceRequest(t, router, http.MethodPost, "/app/devices",
		&pb.DeviceRegisterRequest{Token: "tok", Name: "Box", PackageId: "com.other.app", DomainId: 1})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var device pb.Device
	if err := proto.Unmarshal(w.Body.Bytes(), &device); err != nil {
		t.Fatalf("decode device: %v", err)
	}
	if device.IptvUrl != "" {
		t.Fatalf("iptv_url = %q, want it empty", device.IptvUrl)
	}
}
