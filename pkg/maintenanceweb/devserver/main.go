// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command devserver runs the Nirvati Init UI's static frontend against fake API responses, so it
// can be exercised in a real browser without booting a Talos node.
//
//	go run ./pkg/maintenanceweb/devserver
//
// It is dev tooling only - it does not generate or apply any real config.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/siderolabs/talos/pkg/maintenanceweb"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/system-info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"candidateEndpoints": []string{"192.168.1.42"},
			"hostname":           "nirvati",
		})
	})

	// simulate disk discovery taking a couple of seconds, same as it would on real hardware
	start := time.Now()
	mux.HandleFunc("GET /api/v1/disks", func(w http.ResponseWriter, r *http.Request) {
		if time.Since(start) < 2*time.Second {
			writeJSON(w, []any{})

			return
		}

		writeJSON(w, []map[string]any{
			{
				"devPath":    "/dev/sda",
				"prettySize": "256 GB",
				"model":      "Samsung SSD 870",
				"serial":     "S6BXNX0R123456",
				"transport":  "sata",
				"rotational": false,
			},
		})
	})

	mux.HandleFunc("POST /api/v1/cluster/new", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		log.Printf("cluster/new: %+v", body)
		writeJSON(w, map[string]any{"status": "applying"})
	})

	mux.HandleFunc("POST /api/v1/cluster/join", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck
		log.Printf("cluster/join: %d bytes", len(body))
		writeJSON(w, map[string]any{"status": "applying"})
	})

	mux.Handle("/", maintenanceweb.Handler())

	const addr = "localhost:8080"

	log.Printf("Nirvati Init UI dev server listening on http://%s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil { //nolint:gosec
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck
}
