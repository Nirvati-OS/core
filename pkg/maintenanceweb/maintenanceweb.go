// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package maintenanceweb serves the status page shown on port 80 while a machine is uninitialized (in maintenance mode).
//
// To customize the page, edit the files under static/ (index.html, favicon.ico, logo.svg, ...) - anything placed
// there is served as-is at the matching path, no code changes required.
package maintenanceweb

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// Handler returns an http.Handler serving the maintenance status page and its static assets.
func Handler() http.Handler {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	return http.FileServer(http.FS(static))
}
