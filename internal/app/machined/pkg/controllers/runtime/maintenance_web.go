// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/gen/optional"
	"go.uber.org/zap"

	machinedruntime "github.com/siderolabs/talos/internal/app/machined/pkg/runtime"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"github.com/siderolabs/talos/pkg/maintenanceweb"
)

// MaintenanceWebController runs an HTTP server on constants.MaintenanceWebPort while the machine is uninitialized,
// serving the Nirvati Init UI (see pkg/maintenanceweb) and its backing API (see maintenance_web_api.go).
type MaintenanceWebController struct {
	V1Alpha1Runtime machinedruntime.Runtime
}

// Name implements controller.Controller interface.
func (ctrl *MaintenanceWebController) Name() string {
	return "runtime.MaintenanceWebController"
}

// Inputs implements controller.Controller interface.
func (ctrl *MaintenanceWebController) Inputs() []controller.Input {
	return []controller.Input{
		{
			Namespace: runtime.NamespaceName,
			Type:      runtime.MaintenanceServiceRequestType,
			ID:        optional.Some(runtime.MaintenanceServiceRequestID),
			Kind:      controller.InputWeak,
		},
	}
}

// Outputs implements controller.Controller interface.
func (ctrl *MaintenanceWebController) Outputs() []controller.Output {
	return nil
}

// Run implements controller.Controller interface.
func (ctrl *MaintenanceWebController) Run(ctx context.Context, r controller.Runtime, logger *zap.Logger) error {
	var stopServer func()

	defer func() {
		if stopServer != nil {
			stopServer()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.EventCh():
		}

		request, err := safe.ReaderGetByID[*runtime.MaintenanceServiceRequest](ctx, r, runtime.MaintenanceServiceRequestID)
		if err != nil && !state.IsNotFoundError(err) {
			return fmt.Errorf("failed to get maintenance service request: %w", err)
		}

		switch {
		case request != nil && stopServer == nil:
			if stopServer, err = ctrl.startServer(logger); err != nil {
				return fmt.Errorf("failed to start maintenance web server: %w", err)
			}
		case request == nil && stopServer != nil:
			stopServer()
			stopServer = nil
		}

		r.ResetRestartBackoff()
	}
}

func (ctrl *MaintenanceWebController) startServer(logger *zap.Logger) (func(), error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", constants.MaintenanceWebPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", constants.MaintenanceWebPort, err)
	}

	api := &maintenanceWebAPI{runtime: ctrl.V1Alpha1Runtime}
	mux := api.mux()
	mux.Handle("/", maintenanceweb.Handler())

	httpServer := &http.Server{
		Handler: mux,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("maintenance web server failed", zap.Error(err))
		}
	}()

	logger.Info("started maintenance web server", zap.Stringer("address", listener.Addr()))

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("failed to shut down maintenance web server", zap.Error(err))
		}
	}, nil
}
