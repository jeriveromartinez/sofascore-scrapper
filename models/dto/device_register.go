package dto

type DeviceRegisterRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
	Name     string `json:"name"`
}
