package video

import "time"

type Video struct {
	ID                string `json:"video_id" db:"video_id"`
	Name              string `json:"name" db:"name" `
	HLS               bool   `json:"hls" db:"hls"`
	Description       string `json:"description" db:"description"`
	Preview           string `json:"preview" db:"preview"`
	ThumbImage        string `json:"thumb_image" db:"thumb_image"`
	Src               string `json:"src" db:"src"`
	DownloadedFromURL string `json:"downloaded_from_url" db:"downloaded_from_url"`
	Dir               string `json:"dir" db:"dir"`
	Length            int    `json:"length" db:"length"`

	Views int `json:"views" db:"views"`

	UpdatedTimestamp time.Time `db:"updated_timestamp" json:"updated_timestamp"`
	CreatedTimestamp time.Time `db:"created_timestamp" json:"created_timestamp"`
}
