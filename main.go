package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AircraftState struct {
	Hex          string  `json:"hex"`
	Callsign     string  `json:"callsign,omitempty"`
	Lat          float64 `json:"lat,omitempty"`
	Lon          float64 `json:"lon,omitempty"`
	Altitude     int     `json:"altitude,omitempty"`
	Speed        int     `json:"speed,omitempty"`
	Heading      float64 `json:"heading,omitempty"`
	VerticalRate int     `json:"vertical_rate,omitempty"`
	LastSeen     time.Time `json:"-"`
}

type pushPayload struct {
	SourceID string          `json:"source_id"`
	Aircraft []AircraftState `json:"aircraft"`
}

var (
	sbsAddr  = flag.String("sbs", "localhost:30003", "SBS host:port")
	server   = flag.String("server", "http://awoslog.com", "awoslog server URL")
	source   = flag.String("source", "stratux-home", "source identifier")
	interval = flag.Duration("interval", 3*time.Second, "push interval")
	apiKey   = flag.String("key", "", "optional API key")

	mu       sync.Mutex
	aircraft = make(map[string]*AircraftState)
	client   = &http.Client{Timeout: 10 * time.Second}
)

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("stratux-pusher starting: sbs=%s server=%s source=%s interval=%s",
		*sbsAddr, *server, *source, *interval)

	go readSBS()
	go cleanupLoop()
	pushLoop()
}

// readSBS connects to the SBS port and parses messages continuously.
func readSBS() {
	backoff := 5 * time.Second
	maxBackoff := 30 * time.Second

	for {
		conn, err := net.Dial("tcp", *sbsAddr)
		if err != nil {
			log.Printf("SBS connect error: %v (retry in %s)", err, backoff)
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff = time.Duration(float64(backoff) * 1.5)
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		log.Printf("connected to SBS at %s", *sbsAddr)
		backoff = 5 * time.Second

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			parseSBSLine(scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			log.Printf("SBS read error: %v", err)
		} else {
			log.Printf("SBS connection closed")
		}
		conn.Close()
	}
}

// parseSBSLine parses a single SBS/BaseStation CSV line and updates aircraft state.
func parseSBSLine(line string) {
	fields := strings.Split(line, ",")
	if len(fields) < 11 || fields[0] != "MSG" {
		return
	}

	msgType := fields[1]
	hex := strings.ToUpper(strings.TrimSpace(fields[4]))
	if hex == "" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	ac, ok := aircraft[hex]
	if !ok {
		ac = &AircraftState{Hex: hex}
		aircraft[hex] = ac
	}
	ac.LastSeen = time.Now()

	switch msgType {
	case "1":
		// ES Identification: field[10] = callsign
		if len(fields) > 10 {
			cs := strings.TrimSpace(fields[10])
			if cs != "" {
				ac.Callsign = cs
			}
		}
	case "3":
		// ES Airborne Position: field[11]=altitude, field[14]=lat, field[15]=lon
		if len(fields) > 15 {
			if alt, err := strconv.Atoi(strings.TrimSpace(fields[11])); err == nil {
				ac.Altitude = alt
			}
			if lat, err := strconv.ParseFloat(strings.TrimSpace(fields[14]), 64); err == nil {
				ac.Lat = lat
			}
			if lon, err := strconv.ParseFloat(strings.TrimSpace(fields[15]), 64); err == nil {
				ac.Lon = lon
			}
		}
	case "4":
		// ES Airborne Velocity: field[12]=speed, field[13]=heading, field[16]=vert_rate
		if len(fields) > 16 {
			if spd, err := strconv.ParseFloat(strings.TrimSpace(fields[12]), 64); err == nil {
				ac.Speed = int(math.Round(spd))
			}
			if hdg, err := strconv.ParseFloat(strings.TrimSpace(fields[13]), 64); err == nil {
				ac.Heading = hdg
			}
			if vr, err := strconv.Atoi(strings.TrimSpace(fields[16])); err == nil {
				ac.VerticalRate = vr
			}
		}
	case "5", "7":
		// Altitude updates: field[11]=altitude
		if len(fields) > 11 {
			if alt, err := strconv.Atoi(strings.TrimSpace(fields[11])); err == nil {
				ac.Altitude = alt
			}
		}
	}
}

// pushLoop sends the current aircraft state to the server at the configured interval.
func pushLoop() {
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for range ticker.C {
		snapshot := getSnapshot()
		if len(snapshot) == 0 {
			continue
		}
		push(snapshot)
	}
}

// getSnapshot returns a copy of all tracked aircraft.
func getSnapshot() []AircraftState {
	mu.Lock()
	defer mu.Unlock()

	result := make([]AircraftState, 0, len(aircraft))
	for _, ac := range aircraft {
		result = append(result, *ac)
	}
	return result
}

// push sends the aircraft snapshot to the awoslog server.
func push(snapshot []AircraftState) {
	payload := pushPayload{
		SourceID: *source,
		Aircraft: snapshot,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}

	url := fmt.Sprintf("%s/api/stratux/push", *server)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		log.Printf("request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if *apiKey != "" {
		req.Header.Set("X-Stratux-Key", *apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("push error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("push failed: HTTP %d", resp.StatusCode)
	}
}

// cleanupLoop removes aircraft not seen for 60 seconds.
func cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		now := time.Now()
		for hex, ac := range aircraft {
			if now.Sub(ac.LastSeen) > 60*time.Second {
				delete(aircraft, hex)
			}
		}
		mu.Unlock()
	}
}
