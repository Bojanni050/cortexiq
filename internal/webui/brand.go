// Copyright 2025 Bojan van der Heide
// SPDX-License-Identifier: Apache-2.0

package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// serveBrand serves the CortexIQ brand assets — currently the brain mark in two
// sizes — from the embedded filesystem. Keeping them embedded means the
// dashboard leaves the network alone and the mark never 404s on a fresh
// install, which is how the old favicon behaved.
func (h *Handler) serveBrand(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.URL.Path)
	if name == "." || name == "/" || !strings.HasSuffix(name, ".png") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(staticFS, "static/brand/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// The file names carry no version; a release is what changes them, so a
	// day of caching is cheap and saves the repeated favicon fetch.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
