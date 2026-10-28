package totem

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTConfig struct {
	BrokerURL string // contoh: tcp://127.0.0.1:1883
	User      string
	Password  string
}

type MQTTHub struct {
	client mqtt.Client

	mu     sync.RWMutex
	states map[string]*Status

	observer func(deviceID string, status Status)
}

// NewMQTTHub tidak menunggu broker: bila broker belum hidup, sambungan dicoba ulang di latar belakang.
func NewMQTTHub(cfg MQTTConfig) *MQTTHub {
	hub := &MQTTHub{states: map[string]*Status{}}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID("talepuff-backend").
		SetUsername(cfg.User).
		SetPassword(cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT tersambung ke", cfg.BrokerURL)
		for _, kind := range []string{"status", "online"} {
			topic := "talepuff/+/" + kind
			if t := c.Subscribe(topic, 1, hub.onMessage); t.Wait() && t.Error() != nil {
				log.Printf("MQTT: gagal berlangganan %s: %v", topic, t.Error())
			}
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) { log.Println("MQTT terputus:", err) })

	hub.client = mqtt.NewClient(opts)
	hub.client.Connect()
	return hub
}

func (h *MQTTHub) onMessage(_ mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) != 3 || !ValidDeviceID(parts[1]) {
		return
	}
	id, kind := parts[1], parts[2]

	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.states[id]
	if state == nil {
		state = &Status{State: "unknown"}
		h.states[id] = state
	}
	now := time.Now()
	state.UpdatedAt = &now

	switch kind {
	case "online":
		state.Online = string(msg.Payload()) == "1"
		if !state.Online {
			state.State = "offline"
		}
	case "status":
		var report struct {
			State    string  `json:"state"`
			Reason   *string `json:"reason"`
			StoryID  *uint   `json:"story_id"`
			Position int     `json:"position"`
			Duration int     `json:"duration"`
			Volume   int     `json:"volume"`
		}
		if err := json.Unmarshal(msg.Payload(), &report); err != nil {
			return
		}
		state.Online = true
		state.State, state.Reason, state.StoryID = report.State, report.Reason, report.StoryID
		state.Position, state.Duration, state.Volume = report.Position, report.Duration, report.Volume
		if h.observer != nil {
			snapshot := *state
			go h.observer(id, snapshot)
		}
	}
}

// OnStatus mendaftarkan fungsi yang dipanggil setiap kali totem melaporkan status. Panggil sebelum dipakai.
func (h *MQTTHub) OnStatus(fn func(deviceID string, status Status)) {
	h.mu.Lock()
	h.observer = fn
	h.mu.Unlock()
}

func (h *MQTTHub) Send(deviceID string, cmd Command) error {
	if !ValidDeviceID(deviceID) {
		return fmt.Errorf("invalid device id")
	}
	if !h.client.IsConnectionOpen() {
		return ErrNotConnected
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	token := h.client.Publish("talepuff/"+deviceID+"/cmd", 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return ErrNotConnected
	}
	return token.Error()
}

func (h *MQTTHub) Status(deviceID string) Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if state := h.states[deviceID]; state != nil {
		return *state
	}
	return Status{State: "unknown"}
}

func (h *MQTTHub) Known(deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.states[deviceID]
	return ok
}

func (h *MQTTHub) Close() { h.client.Disconnect(250) }
