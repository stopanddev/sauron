// Package server is the HTTP UI for the Sauron hub.
//
// HTMX: layout uses hx-boost and fragment routes (e.g. /partials/status). When adding
// browser auth, mirror Tiamat: respond to HX-Request with HX-Redirect on 401/403 so
// partial loads navigate correctly.
package server
