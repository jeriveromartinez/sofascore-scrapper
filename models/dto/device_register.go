package dto

type DeviceRegisterRequest struct {
	Token    string `json:"token" cbor:"token"`
	Platform string `json:"platform" cbor:"platform"`
	Name     string `json:"name" cbor:"name"`
}