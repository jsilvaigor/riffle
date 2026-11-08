package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// probeVersion is bumped when Probe gains fields worth re-reading existing files for.
// Items indexed with an older version are probed again on the next scan.
//
//	2: video rotation and display size
const probeVersion = 2

type Probe struct {
	Version   int           `json:"v"`
	Duration  float64       `json:"duration"` // seconds
	Container string        `json:"container"`
	Bitrate   int64         `json:"bitrate"` // bits/s
	Video     *VideoStream  `json:"video,omitempty"`
	Audio     []AudioStream `json:"audio"`
	Subtitles []SubStream   `json:"subtitles"`
}

type VideoStream struct {
	Index     int     `json:"index"`
	Codec     string  `json:"codec"`
	Profile   string  `json:"profile,omitempty"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FrameRate float64 `json:"frameRate"`
	PixFmt    string  `json:"pixFmt,omitempty"`
	// Rotation is how far the stored frame turns clockwise to display: 0, 90, 180 or 270.
	// Phones store most portrait clips as landscape pixels plus a 90 or 270 rotation.
	// ffmpeg, mpv and the trickplay sheets apply it automatically.
	Rotation int `json:"rotation"`
	// DisplayWidth and DisplayHeight are the size the frame is shown at, after the
	// rotation and any non-square pixels. The iOS player picks its orientation from these.
	DisplayWidth  int `json:"displayWidth"`
	DisplayHeight int `json:"displayHeight"`
}

type AudioStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default"`
}

type SubStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
}

type ffprobeOut struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index        int    `json:"index"`
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Profile      string `json:"profile"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		SampleAspect string `json:"sample_aspect_ratio"`
		SideData     []struct {
			Type     string   `json:"side_data_type"`
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
		PixFmt       string            `json:"pix_fmt"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		Channels     int               `json:"channels"`
		Tags         map[string]string `json:"tags"`
		Disposition  map[string]int    `json:"disposition"`
	} `json:"streams"`
}

func parseRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	n, _ := strconv.ParseFloat(num, 64)
	d, _ := strconv.ParseFloat(den, 64)
	if d == 0 {
		return 0
	}
	return n / d
}

// clockwiseRotation normalises a stream's rotation to 0, 90, 180 or 270 degrees clockwise.
// The display matrix side data (ffprobe's "rotation") is counter-clockwise; the older
// "rotate" tag is clockwise. The display matrix wins when both are present.
func clockwiseRotation(displayMatrix *float64, rotateTag string) int {
	deg := 0.0
	switch {
	case displayMatrix != nil:
		deg = -*displayMatrix
	case rotateTag != "":
		deg, _ = strconv.ParseFloat(rotateTag, 64)
	}
	r := int(math.Round(deg/90)) * 90 % 360
	if r < 0 {
		r += 360
	}
	return r
}

// displaySize applies non-square pixels (sample aspect ratio "N:M") and then the rotation.
func displaySize(width, height int, sar string, rotation int) (int, int) {
	w, h := width, height
	if num, den, ok := strings.Cut(sar, ":"); ok {
		n, _ := strconv.Atoi(num)
		d, _ := strconv.Atoi(den)
		if n > 0 && d > 0 && n != d {
			w = int(math.Round(float64(width) * float64(n) / float64(d)))
		}
	}
	if rotation == 90 || rotation == 270 {
		w, h = h, w
	}
	return w, h
}

func probeFile(ctx context.Context, path string) (*Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ffprobe: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var raw ffprobeOut
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe output: %w", err)
	}
	p := &Probe{Version: probeVersion, Container: raw.Format.FormatName, Audio: []AudioStream{}, Subtitles: []SubStream{}}
	p.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	p.Bitrate, _ = strconv.ParseInt(raw.Format.BitRate, 10, 64)
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			// Skip cover art / attached pictures; keep the first real video stream.
			if p.Video != nil || s.Disposition["attached_pic"] == 1 {
				continue
			}
			fr := parseRate(s.AvgFrameRate)
			if fr == 0 {
				fr = parseRate(s.RFrameRate)
			}
			var matrix *float64
			for _, sd := range s.SideData {
				if sd.Type == "Display Matrix" && sd.Rotation != nil {
					matrix = sd.Rotation
				}
			}
			rot := clockwiseRotation(matrix, s.Tags["rotate"])
			dw, dh := displaySize(s.Width, s.Height, s.SampleAspect, rot)
			p.Video = &VideoStream{Index: s.Index, Codec: s.CodecName, Profile: s.Profile,
				Width: s.Width, Height: s.Height, FrameRate: fr, PixFmt: s.PixFmt,
				Rotation: rot, DisplayWidth: dw, DisplayHeight: dh}
		case "audio":
			p.Audio = append(p.Audio, AudioStream{Index: s.Index, Codec: s.CodecName,
				Channels: s.Channels, Language: s.Tags["language"], Title: s.Tags["title"],
				Default: s.Disposition["default"] == 1})
		case "subtitle":
			p.Subtitles = append(p.Subtitles, SubStream{Index: s.Index, Codec: s.CodecName,
				Language: s.Tags["language"], Title: s.Tags["title"],
				Default: s.Disposition["default"] == 1, Forced: s.Disposition["forced"] == 1})
		}
	}
	return p, nil
}
