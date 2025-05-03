package models

type GuildConfig struct {
	GuildID       string
	ChannelID     string
	RoleID        string
	LogsChannelID string
}

type VerifyUser struct {
	RequestID       string
	UserID          string
	RequestTime     int64
	VerifyMessageID string
	VerifyChannelID string
	GuildID         string
}

type VerifyData struct {
	Fingerprint  string `json:"print"`
	CaptchaToken string `json:"token"`
}

type IpInfoBody struct {
	Result string `json:"result"`
}

type UserLog struct {
	Id          string
	RequestId   string
	Fingerprint string
	IPScore     string
	Passed      bool
}
