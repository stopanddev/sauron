package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"sauron/internal/tiamat"
	"sauron/views"
)

// TiamatHostControl runs same-host start/stop commands for Tiamat.
type TiamatHostControl interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	CanStart() bool
	CanStop() bool
}

// Handler serves HTML for the hub.
type Handler struct {
	Client         *tiamat.Client
	TokenSet       bool
	TiamatControl  TiamatHostControl
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(recoverPanic)
	r.Get("/", h.getDashboard)
	r.Get("/partials/status", h.getStatusPartial)
	r.Get("/stats", h.getStats)
	r.Get("/users", h.getUsers)
	r.Post("/users", h.postUsers)
	r.Post("/tiamat/start", h.postTiamatStart)
	r.Post("/tiamat/stop", h.postTiamatStop)
	sub, err := fs.Sub(Static, "static")
	if err != nil {
		panic("static embed: " + err.Error())
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	return r
}

func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusData struct {
	healthy       bool
	healthSnippet string
	hubDisabled   bool
	tokenMissing  bool
	statusErr     string
	version       string
	uptime        string
	dbOk          string
}

func (h *Handler) loadStatus(ctx context.Context, tokenSet bool) statusData {
	var d statusData
	d.tokenMissing = !tokenSet

	ok, snippet, err := h.Client.Healthz(ctx)
	d.healthy = ok
	d.healthSnippet = snippet
	if err != nil {
		d.healthSnippet = err.Error()
	}

	if d.tokenMissing {
		return d
	}

	st, err := h.Client.Status(ctx)
	if errors.Is(err, tiamat.ErrHubDisabled) {
		d.hubDisabled = true
		return d
	}
	if err != nil {
		d.statusErr = err.Error()
		return d
	}
	d.version = st.Version
	d.uptime = formatDuration(st.UptimeSeconds)
	if st.DbOk {
		d.dbOk = "yes"
	} else {
		d.dbOk = "no"
	}
	return d
}

func formatDuration(sec float64) string {
	if sec <= 0 {
		return "0s"
	}
	d := time.Duration(sec * float64(time.Second))
	return d.Round(time.Second).String()
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	d := h.loadStatus(r.Context(), h.TokenSet)
	note, noteErr := controlFlashFromQuery(r.URL.Query())
	canStart, canStop := h.tiamatCanStartStop()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.DashboardPage(
		d.healthy,
		d.healthSnippet,
		d.hubDisabled,
		d.tokenMissing,
		d.statusErr,
		d.version,
		d.uptime,
		d.dbOk,
		canStart,
		canStop,
		note,
		noteErr,
	).Render(r.Context(), w)
}

func (h *Handler) getStatusPartial(w http.ResponseWriter, r *http.Request) {
	d := h.loadStatus(r.Context(), h.TokenSet)
	canStart, canStop := h.tiamatCanStartStop()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.StatusFragment(
		d.healthy,
		d.healthSnippet,
		d.hubDisabled,
		d.tokenMissing,
		d.statusErr,
		d.version,
		d.uptime,
		d.dbOk,
		canStart,
		canStop,
		"",
		false,
	).Render(r.Context(), w)
}

func (h *Handler) tiamatCanStartStop() (canStart, canStop bool) {
	if h.TiamatControl == nil {
		return false, false
	}
	return h.TiamatControl.CanStart(), h.TiamatControl.CanStop()
}

func controlFlashFromQuery(q url.Values) (msg string, isErr bool) {
	if raw := q.Get("stop_err"); raw != "" {
		dec, err := url.QueryUnescape(raw)
		if err != nil {
			dec = raw
		}
		return truncateRunes(dec, 500), true
	}
	switch {
	case q.Get("stop_ok") == "1":
		return "Tiamat /healthz is down (stop completed).", false
	case q.Get("stop_already") == "1":
		return "Tiamat was already stopped.", false
	case q.Get("stop_nc") == "1":
		return "Stop is not configured (set SAURON_TIAMAT_STOP_SCRIPT, or use systemd start so systemctl stop is available).", true
	}
	switch {
	case q.Get("start_ok") == "1":
		return "Tiamat is responding on /healthz.", false
	case q.Get("start_already") == "1":
		return "Tiamat was already running.", false
	case q.Get("start_nc") == "1":
		return "Start is not configured (set SAURON_TIAMAT_START_SCRIPT or SAURON_TIAMAT_SYSTEMD_UNIT).", true
	}
	raw := q.Get("start_err")
	if raw == "" {
		return "", false
	}
	dec, err := url.QueryUnescape(raw)
	if err != nil {
		dec = raw
	}
	return truncateRunes(dec, 500), true
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func (h *Handler) postTiamatStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if h.TiamatControl == nil || !h.TiamatControl.CanStart() {
		http.Redirect(w, r, "/?start_nc=1", http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	if ok, _, _ := h.Client.Healthz(ctx); ok {
		http.Redirect(w, r, "/?start_already=1", http.StatusSeeOther)
		return
	}

	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := h.TiamatControl.Start(startCtx)
	cancel()
	if err != nil {
		http.Redirect(w, r, "/?start_err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	pollDeadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(pollDeadline) {
		pollCtx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
		ok, _, _ := h.Client.Healthz(pollCtx)
		pcancel()
		if ok {
			http.Redirect(w, r, "/?start_ok=1", http.StatusSeeOther)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	http.Redirect(w, r, "/?start_err="+url.QueryEscape("start command ran but /healthz did not become ready in time"), http.StatusSeeOther)
}

func (h *Handler) postTiamatStop(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if h.TiamatControl == nil || !h.TiamatControl.CanStop() {
		http.Redirect(w, r, "/?stop_nc=1", http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	if ok, _, _ := h.Client.Healthz(ctx); !ok {
		http.Redirect(w, r, "/?stop_already=1", http.StatusSeeOther)
		return
	}

	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := h.TiamatControl.Stop(stopCtx)
	cancel()
	if err != nil {
		http.Redirect(w, r, "/?stop_err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	pollDeadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(pollDeadline) {
		pollCtx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
		ok, _, _ := h.Client.Healthz(pollCtx)
		pcancel()
		if !ok {
			http.Redirect(w, r, "/?stop_ok=1", http.StatusSeeOther)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	http.Redirect(w, r, "/?stop_err="+url.QueryEscape("stop command ran but /healthz still responded"), http.StatusSeeOther)
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	kind := q.Get("kind")
	limitStr := q.Get("limit")
	if limitStr == "" {
		limitStr = "50"
	}
	offsetStr := q.Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}
	loadAssets := q.Get("assets") == "1"

	var summaryJSON, assetsJSON string
	var errMsg string
	hubDisabled := false

	if !h.TokenSet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = views.StatsPage(from, to, kind, limitStr, offsetStr, "", "", "Set TIAMAT_HUB_TOKEN to load stats.", false).Render(ctx, w)
		return
	}

	var fromRFC, toRFC string
	if from != "" && to != "" {
		var e1, e2 error
		fromRFC, e1 = tiamat.FormatHubStatsParam(from, false)
		toRFC, e2 = tiamat.FormatHubStatsParam(to, true)
		if e1 != nil {
			errMsg = e1.Error()
		} else if e2 != nil {
			errMsg = e2.Error()
		}
	}

	if errMsg == "" && from != "" && to != "" {
		raw, err := h.Client.StatsSummary(ctx, fromRFC, toRFC)
		if errors.Is(err, tiamat.ErrHubDisabled) {
			hubDisabled = true
		} else if err != nil {
			errMsg = err.Error()
		} else {
			summaryJSON = prettyJSON(raw)
		}
	}

	if loadAssets && from != "" && to != "" && !hubDisabled && errMsg == "" {
		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)
		raw, err := h.Client.StatsAssets(ctx, fromRFC, toRFC, kind, limit, offset)
		if errors.Is(err, tiamat.ErrHubDisabled) {
			hubDisabled = true
		} else if err != nil {
			errMsg = err.Error()
		} else {
			assetsJSON = prettyJSON(raw)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.StatsPage(from, to, kind, limitStr, offsetStr, summaryJSON, assetsJSON, errMsg, hubDisabled).Render(ctx, w)
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func (h *Handler) getUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	createErr := r.URL.Query().Get("err")
	createOk := r.URL.Query().Get("ok")
	patchErr := r.URL.Query().Get("perr")

	hubDisabled := false
	var rows []views.UserRow
	if !h.TokenSet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = views.UsersPage(nil, createErr, createOk, patchErr, false, true).Render(ctx, w)
		return
	}
	list, err := h.Client.ListUsers(ctx)
	if errors.Is(err, tiamat.ErrHubDisabled) {
		hubDisabled = true
	} else if err != nil {
		createErr = err.Error()
	} else {
		for _, u := range list {
			rows = append(rows, views.UserRow{
				Username: u.Username,
				IsAdmin:  u.IsAdmin,
				Disabled: u.Disabled,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.UsersPage(rows, createErr, createOk, patchErr, hubDisabled, false).Render(ctx, w)
}

func (h *Handler) postUsers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !h.TokenSet {
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	action := r.FormValue("action")
	switch action {
	case "create":
		u := r.FormValue("username")
		p := r.FormValue("password")
		body := tiamat.CreateUserBody{Username: u, Password: p}
		if r.FormValue("is_admin") == "true" {
			t := true
			body.IsAdmin = &t
		}
		if err := h.Client.CreateUser(ctx, body); err != nil {
			if errors.Is(err, tiamat.ErrHubDisabled) {
				http.Redirect(w, r, "/users?err="+url.QueryEscape("hub API disabled on Tiamat"), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/users?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/users?ok="+url.QueryEscape("created "+u), http.StatusSeeOther)
	case "patch":
		un := r.FormValue("username")
		pw := r.FormValue("password")
		isAdmin := r.FormValue("is_admin") == "true"
		disabled := r.FormValue("disabled") == "true"
		var b tiamat.PatchUserBody
		if pw != "" {
			b.Password = &pw
		}
		b.IsAdmin = &isAdmin
		b.Disabled = &disabled
		if err := h.Client.PatchUser(ctx, un, b); err != nil {
			if errors.Is(err, tiamat.ErrHubDisabled) {
				http.Redirect(w, r, "/users?perr="+url.QueryEscape("hub API disabled"), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/users?perr="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/users", http.StatusSeeOther)
	case "delete":
		un := r.FormValue("username")
		if err := h.Client.DeleteUser(ctx, un); err != nil {
			if errors.Is(err, tiamat.ErrHubDisabled) {
				http.Redirect(w, r, "/users?perr="+url.QueryEscape("hub API disabled"), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/users?perr="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/users?ok="+url.QueryEscape("disabled "+un), http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/users", http.StatusSeeOther)
	}
}

