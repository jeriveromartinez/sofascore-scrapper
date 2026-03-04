// Package apkutil extracts metadata from Android APK files.
package apkutil

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/avast/apkparser"
)

// APKInfo holds metadata extracted from an Android APK's AndroidManifest.xml.
type APKInfo struct {
	PackageName      string
	VersionName      string
	VersionCode      int32
	MinSDKVersion    int32
	TargetSDKVersion int32
}

const androidNS = "http://schemas.android.com/apk/res/android"

// ParseAPKInfo opens the APK at filePath and extracts key manifest metadata.
func ParseAPKInfo(filePath string) (*APKInfo, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	_, _, manifestErr := apkparser.ParseApk(filePath, enc)
	if manifestErr != nil {
		return nil, fmt.Errorf("failed to parse AndroidManifest.xml: %w", manifestErr)
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush XML encoder: %w", err)
	}

	return extractManifestInfo(&buf)
}

// extractManifestInfo reads the XML manifest and extracts APK metadata.
func extractManifestInfo(r io.Reader) (*APKInfo, error) {
	info := &APKInfo{}
	dec := xml.NewDecoder(r)

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error decoding manifest XML: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "manifest":
			for _, attr := range start.Attr {
				switch {
				// The "package" attribute on the manifest element carries no namespace prefix in the APK binary XML.
				case attr.Name.Space == "" && attr.Name.Local == "package":
					info.PackageName = attr.Value
				case attr.Name.Space == androidNS && attr.Name.Local == "versionName":
					info.VersionName = attr.Value
				case attr.Name.Space == androidNS && attr.Name.Local == "versionCode":
					n, err := strconv.ParseInt(attr.Value, 10, 32)
					if err == nil {
						info.VersionCode = int32(n)
					}
				}
			}
		case "uses-sdk":
			for _, attr := range start.Attr {
				switch {
				case attr.Name.Space == androidNS && attr.Name.Local == "minSdkVersion":
					n, err := strconv.ParseInt(attr.Value, 10, 32)
					if err == nil {
						info.MinSDKVersion = int32(n)
					}
				case attr.Name.Space == androidNS && attr.Name.Local == "targetSdkVersion":
					n, err := strconv.ParseInt(attr.Value, 10, 32)
					if err == nil {
						info.TargetSDKVersion = int32(n)
					}
				}
			}
		}
	}

	if info.PackageName == "" {
		return nil, errors.New("could not extract package name from APK manifest")
	}
	return info, nil
}
