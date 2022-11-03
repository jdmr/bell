package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hajimehoshi/oto/v2"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

var cronService *cron.Cron

type event struct {
	Time  string `json:"time"`
	Sound string `json:"sound"`
}

type day struct {
	Name   string   `json:"name"`
	Events []*event `json:"events"`
}

type schedule struct {
	Name   string `json:"name"`
	Starts string `json:"starts"`
	Ends   string `json:"ends"`
	Days   []*day `json:"days"`
}

func parseSchedule() error {
	jsonFile, err := os.ReadFile("./schedule.json")
	if err != nil {
		log.Fatalf("Could not open schedule.json: %v", err)
	}

	data := []*schedule{}
	err = json.Unmarshal(jsonFile, &data)
	if err != nil {
		log.Fatalf("Could not parse schedule.json: %v", err)
	}

	if cronService != nil {
		cronService.Stop()
	}
	cronService = cron.New()
	now := time.Now()
	for _, sch := range data {
		starts, err := time.Parse("2006-01-02", sch.Starts)
		if err != nil {
			log.Errorf("Could not parse start date: %s : %v", sch.Starts, err)
			continue
		}
		ends, err := time.Parse("2006-01-02", sch.Ends)
		if err != nil {
			log.Errorf("Could not parse start date: %s : %v", sch.Starts, err)
			continue
		}
		if now.Before(starts) {
			continue
		}
		if now.After(ends) {
			continue
		}

		log.Printf("Configuring schedule: %s", sch.Name)
		err = configureDays(sch.Days)
		if err != nil {
			log.Errorf("Could not configure days: %v", err)
		}
		cronService.AddFunc("1 0 * * *", func() {
			parseSchedule()
		})
		cronService.Start()
	}

	return nil
}

func configureDays(days []*day) error {
	for _, d := range days {
		name := strings.ToUpper(d.Name[0:3])
		err := configureEvents(name, d.Events)
		if err != nil {
			log.Errorf("Could not configure events: %v", err)
		}
	}
	return nil
}

func configureEvents(dayName string, events []*event) error {
	log.Printf("Configuring: %s", dayName)
	for _, evt := range events {
		hour, err := strconv.Atoi(evt.Time[0:2])
		if err != nil {
			log.Errorf("Could not parse hour: %s : %v", evt.Time[0:2], err)
			continue
		}
		minute, err := strconv.Atoi(evt.Time[3:])
		if err != nil {
			log.Errorf("Could not parse minute: %s : %v", evt.Time[3:], err)
			continue
		}
		time := fmt.Sprintf("%d %d * * %s", minute, hour, dayName)
		log.Printf("%d : %d | %s", hour, minute, time)

		cronService.AddFunc(time, func() {
			playSound(evt.Sound)
		})
	}
	return nil
}

func playSound(sound string) {
	log.Printf("Playing: %s", sound)
	fileBytes, err := os.ReadFile("./sounds/" + sound)
	if err != nil {
		log.Errorf("Could not load audio file: %v", err)
		return
	}
	fileBytesReader := bytes.NewReader(fileBytes)
	decodedMp3, err := mp3.NewDecoder(fileBytesReader)
	if err != nil {
		log.Errorf("Could not decode mp3: %v", err)
		return
	}

	samplingRate := 44100

	// Number of channels (aka locations) to play sounds from. Either 1 or 2.
	// 1 is mono sound, and 2 is stereo (most speakers are stereo).
	numOfChannels := 2

	// Bytes used by a channel to represent one sample. Either 1 or 2 (usually 2).
	audioBitDepth := 2

	// Remember that you should **not** create more than one context
	otoCtx, readyChan, err := oto.NewContext(samplingRate, numOfChannels, audioBitDepth)
	if err != nil {
		log.Errorf("Could not initialize oto: %v", err)
		return
	}

	// It might take a bit for the hardware audio devices to be ready, so we wait on the channel.
	<-readyChan

	// Create a new 'player' that will handle our sound. Paused by default.
	player := otoCtx.NewPlayer(decodedMp3)

	// Play starts playing the sound and returns without waiting for it (Play() is async).
	player.Play()

	// We can wait for the sound to finish playing using something like this
	for player.IsPlaying() {
		time.Sleep(time.Millisecond * 50)
	}

	err = player.Close()
	if err != nil {
		log.Errorf("Could not close player: %v", err)
	}
}
