package video

import (
	"errors"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Video struct {
	ID                string   `json:"video_id" db:"video_id"`
	Name              string   `json:"name" db:"name" `
	HLS               bool     `json:"hls" db:"hls"`
	Description       string   `json:"description" db:"description"`
	Preview           string   `json:"preview" db:"preview"`
	ThumbImage        string   `json:"thumb_image" db:"thumb_image"`
	Src               string   `json:"src" db:"src"`
	DownloadedFromURL string   `json:"downloaded_from_url" db:"downloaded_from_url"`
	Dir               string   `json:"dir" db:"dir"`
	Length            int      `json:"length" db:"length"`
	Tags              []string `json:"tags" bson:"tags"`
	Views             int      `json:"views" db:"views"`

	UpdatedTimestamp time.Time `db:"updated_timestamp" json:"updated_timestamp"`
	CreatedTimestamp time.Time `db:"created_timestamp" json:"created_timestamp"`
}

func (v *Video) HLSHandler(pathPrefix, baseDir string, w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(pathPrefix, "/") {
		pathPrefix = pathPrefix + "/"
	}
	// 3. Build the full on-disk path
	relPath := strings.TrimPrefix(r.URL.Path, pathPrefix)
	fullPath := filepath.Join(baseDir, relPath)

	// 4. Serve the file (playlist or .ts segment)
	http.ServeFile(w, r, fullPath)
}

func (v *Video) PreviewHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(v.Preview); errors.Is(err, os.ErrNotExist) {
		err = CreatePreview(v.Src, v.Preview, 10)
		if err != nil {
			http.Error(w, "Could not create preview file.", http.StatusInternalServerError)
			return
		}
	}
	f, err := os.Open(v.Preview)
	if err != nil {
		http.Error(w, "File not found.", http.StatusNotFound)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "Could not obtain file info.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeContent(w, r, filepath.Base(v.Preview), fi.ModTime(), f)
}

func (v *Video) ThumbHandler(w http.ResponseWriter, r *http.Request) {
	if regen, _ := strconv.ParseBool(r.URL.Query().Get("regen")); regen {
		mp4 := strings.ReplaceAll(v.ThumbImage, "/preview.jpg", ".mp4")
		dir := strings.ReplaceAll(v.ThumbImage, "preview.jpg", "")
		d, err := time.ParseDuration(r.URL.Query().Get("duration"))
		if err == nil {
			_ = generatePreview(mp4, dir, d)
		} else {
			i := rand.Intn(45) + 1
			_ = generatePreview(mp4, dir, time.Duration(i)*time.Second)

		}
		// allow custom timestamp via ?t= in format "00:00:05" or seconds "5"
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400") // cache for 1 day
	http.ServeFile(w, r, v.ThumbImage)
}
