package main

import (
	"encoding/json"
	"io"
	"net/http"
	"pairshell/internal/localui"
	"strconv"
)

func (d *desktopServer) serveHistory(w http.ResponseWriter, r *http.Request) {
	if d.history == nil {
		http.Error(w, "历史记录暂不可用", 503)
		return
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/history":
			list, e := d.history.List(r.URL.Query().Get("q"), r.URL.Query().Get("trash") == "1")
			if e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			json.NewEncoder(w).Encode(list)
		case "/history/entry":
			entry, e := d.history.Get(r.URL.Query().Get("id"))
			if e != nil {
				http.Error(w, "记录不存在", 404)
				return
			}
			json.NewEncoder(w).Encode(entry)
		case "/history/events":
			after, e := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
			if e != nil {
				http.Error(w, "invalid cursor", 400)
				return
			}
			path, e := d.history.EventsPath(r.URL.Query().Get("id"))
			if e != nil {
				json.NewEncoder(w).Encode([]localui.Event{})
				return
			}
			events, e := localui.ReadAuditEvents(path, after)
			if e != nil {
				http.Error(w, "读取记录失败", 500)
				return
			}
			json.NewEncoder(w).Encode(events)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != "POST" || r.Header.Get("Origin") != "http://"+d.host {
		http.Error(w, "forbidden", 403)
		return
	}
	var body struct {
		ID, Title, Note string
		Deleted         bool
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12000))
	dec.DisallowUnknownFields()
	var extra any
	if dec.Decode(&body) != nil || dec.Decode(&extra) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	switch r.URL.Path {
	case "/history/create":
		v, e := d.history.Create(body.Title, body.Note)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		json.NewEncoder(w).Encode(v)
	case "/history/update":
		v, e := d.history.Update(body.ID, body.Title, body.Note)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		json.NewEncoder(w).Encode(v)
	case "/history/delete":
		if e := d.history.Delete(body.ID, body.Deleted); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		io.WriteString(w, "{}")
	default:
		http.NotFound(w, r)
	}
}
