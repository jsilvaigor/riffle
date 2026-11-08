package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Probe struct {
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
		Index        int               `json:"index"`
		CodecType    string            `json:"codec_type"`
		CodecName    string            `json:"codec_name"`
		Profile      string            `json:"profile"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
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
	p := &Probe{Container: raw.Format.FormatName, Audio: []AudioStream{}, Subtitles: []SubStream{}}
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
			p.Video = &VideoStream{Index: s.Index, Codec: s.CodecName, Profile: s.Profile,
				Width: s.Width, Height: s.Height, FrameRate: fr, PixFmt: s.PixFmt}
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
