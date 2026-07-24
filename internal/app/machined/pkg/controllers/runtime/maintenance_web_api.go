// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"

	machinedruntime "github.com/siderolabs/talos/internal/app/machined/pkg/runtime"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/resources/block"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
)

// maintenanceWebAPI implements the Nirvati Init UI's JSON backend: listing install disks and
// candidate endpoint addresses, and applying a machine config generated for a new single-node
// cluster or pasted/uploaded by the user to join an existing one.
type maintenanceWebAPI struct {
	runtime machinedruntime.Runtime

	// mu serializes the generate/validate/apply sequence across concurrent requests to this
	// server. It does not (and cannot) serialize against a concurrent talosctl apply-config
	// call over the maintenance gRPC API, which has its own independent lock - that cross-path
	// race is a pre-existing characteristic of Talos maintenance mode, not something new here.
	mu sync.Mutex
}

func (a *maintenanceWebAPI) mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/system-info", a.handleSystemInfo)
	mux.HandleFunc("GET /api/v1/disks", a.handleListDisks)
	mux.HandleFunc("POST /api/v1/cluster/new", a.handleClusterNew)
	mux.HandleFunc("POST /api/v1/cluster/join", a.handleClusterJoin)

	return mux
}

func (a *maintenanceWebAPI) resources() state.State {
	return a.runtime.State().V1Alpha2().Resources()
}

type diskInfo struct {
	DevPath    string `json:"devPath"`
	PrettySize string `json:"prettySize"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	Transport  string `json:"transport"`
	Rotational bool   `json:"rotational"`
}

func (a *maintenanceWebAPI) handleListDisks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	disks, err := safe.StateListAll[*block.Disk](ctx, a.resources())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)

		return
	}

	var systemDiskID string

	systemDisk, err := safe.StateGetByID[*block.SystemDisk](ctx, a.resources(), block.SystemDiskID)

	switch {
	case err == nil:
		systemDiskID = systemDisk.TypedSpec().DiskID
	case state.IsNotFoundError(err):
		// no system disk yet (e.g. booted from a live ISO with no disk chosen) - nothing to exclude
	default:
		writeError(w, http.StatusInternalServerError, err)

		return
	}

	result := make([]diskInfo, 0, disks.Len())

	for disk := range disks.All() {
		spec := disk.TypedSpec()

		if spec.CDROM || spec.Readonly || disk.Metadata().ID() == systemDiskID {
			continue
		}

		result = append(result, diskInfo{
			DevPath:    spec.DevPath,
			PrettySize: spec.PrettySize,
			Model:      spec.Model,
			Serial:     spec.Serial,
			Transport:  spec.Transport,
			Rotational: spec.Rotational,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

type systemInfo struct {
	CandidateEndpoints []string `json:"candidateEndpoints"`
	Hostname           string   `json:"hostname"`
}

func (a *maintenanceWebAPI) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	info := systemInfo{}

	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}

	nodeAddress, err := safe.StateGetByID[*network.NodeAddress](ctx, a.resources(), network.NodeAddressCurrentID)

	switch {
	case err == nil:
		for _, ip := range nodeAddress.TypedSpec().IPs() {
			if network.NotSideroLinkIP(ip) {
				info.CandidateEndpoints = append(info.CandidateEndpoints, ip.String())
			}
		}
	case state.IsNotFoundError(err):
		// no addresses discovered yet
	default:
		writeError(w, http.StatusInternalServerError, err)

		return
	}

	writeJSON(w, http.StatusOK, info)
}

type clusterNewRequest struct {
	ClusterName string `json:"clusterName"`
	InstallDisk string `json:"installDisk"`
	EndpointIP  string `json:"endpointIP"`
}

func (a *maintenanceWebAPI) handleClusterNew(w http.ResponseWriter, r *http.Request) {
	var req clusterNewRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	// clusterName is optional and inconsequential for a single-node setup - default it rather
	// than bother the user, matching the frontend's own default.
	if req.ClusterName == "" {
		req.ClusterName = "nirvati"
	}

	if req.InstallDisk == "" || req.EndpointIP == "" {
		writeError(w, http.StatusBadRequest, errors.New("installDisk and endpointIP are required"))

		return
	}

	if !a.mu.TryLock() {
		writeError(w, http.StatusConflict, errors.New("a configuration is already being applied"))

		return
	}
	defer a.mu.Unlock()

	cfg, err := GenerateSingleNodeConfig(req.ClusterName, req.InstallDisk, req.EndpointIP)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	a.applyConfig(w, r, cfg)
}

func (a *maintenanceWebAPI) handleClusterJoin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	if !a.mu.TryLock() {
		writeError(w, http.StatusConflict, errors.New("a configuration is already being applied"))

		return
	}
	defer a.mu.Unlock()

	cfg, err := configloader.NewFromBytes(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	a.applyConfig(w, r, cfg)
}

// applyConfig mirrors the steps of the ApplyConfiguration gRPC handler
// (internal/app/machined/internal/server/v1alpha1/v1alpha1_server.go): validate, then persist.
func (a *maintenanceWebAPI) applyConfig(w http.ResponseWriter, r *http.Request, cfg config.Provider) {
	ctx := r.Context()

	warnings, err := cfg.ValidateAtRuntime(ctx, a.resources(), a.runtime.State().Platform().Mode())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)

		return
	}

	if err := a.runtime.SetPersistedConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err)

		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "applying",
		"warnings": warnings,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
