package elevenlabs

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

var (
	//go:embed elevenlabs_api_key
	elevenlabsApiKey string
)

type ElevenLabsResponse struct {
	AudioURL string `json:"audio_url"`
}

func init() {
	if len(elevenlabsApiKey) == 0 {
		log.Fatal("elevenlabs_api_key not set")
	}
}

func TextToSpeech(text string) (string, error) {
	log.Printf("Converting text to speech")
	url := "https://api.elevenlabs.io/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM?optimize_streaming_latency=0&output_format=mp3_44100_128"

	requestBody := map[string]interface{}{
		"text":     text,
		"model_id": "eleven_multilingual_v2",
		"voice_settings": map[string]interface{}{
			"stability":         0.5,
			"similarity_boost":  0,
			"style":             0,
			"use_speaker_boost": true,
		},
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", elevenlabsApiKey)
	req.Header.Set("accept", "audio/mpeg")

	client := &http.Client{}
	resp, err := client.Do(req)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code: %s", resp.Status)

	} else if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp("", "tts_audio_*.mp3")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	_, err = tempFile.Write(audioData)
	if err != nil {
		return "", err
	}
	log.Printf("Audio saved")
	return tempFile.Name(), nil
}
