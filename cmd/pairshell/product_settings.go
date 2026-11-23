package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"pairshell/internal/config"
	"pairshell/internal/paircode"
	"time"
)

func (d *desktopServer) serveSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cfg, e := config.LoadDefault()
		if e != nil {
			http.Error(w, "读取服务器配置失败", 500)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Server            string `json:"server"`
			ForceCustom       bool   `json:"force_custom"`
			OfficialAvailable bool   `json:"official_available"`
			OfficialServer    string `json:"official_server"`
		}{cfg.Server, cfg.ForceCustom, config.OfficialServer != "", config.OfficialServer})
		return
	}
	if r.Method != "POST" || r.Header.Get("Origin") != "http://"+d.host {
		http.Error(w, "forbidden", 403)
		return
	}
	var cfg config.Settings
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	var extra any
	if dec.Decode(&cfg) != nil || dec.Decode(&extra) != io.EOF {
		http.Error(w, "配置格式无效", 400)
		return
	}
	if e := config.UpdatePreferences(cfg.Server, cfg.ForceCustom); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	io.WriteString(w, "{}")
}

func (d *desktopServer) rootContext() context.Context {
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

func (d *desktopServer) beginListener() (context.Context, uint64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil, 0, false
	}
	if d.listenCancel != nil {
		d.listenCancel()
	}
	d.generation++
	ctx, cancel := context.WithCancel(d.rootContext())
	d.listenCancel = cancel
	d.listening = true
	d.phase = "pairing"
	d.state = "正在生成新的配对码"
	d.pairCode = ""
	d.code = ""
	d.control = ""
	return ctx, d.generation, true
}
func (d *desktopServer) listenForHelper() {
	ctx, g, ok := d.beginListener()
	if ok {
		d.listenGeneration(ctx, g)
	}
}
func (d *desktopServer) listenGeneration(ctx context.Context, g uint64) {
	bases, e := config.ServerCandidates("")
	if e != nil {
		d.deviceErrorFor(g)
		d.mu.Lock()
		if d.generation == g && !d.started {
			d.state = e.Error()
		}
		d.mu.Unlock()
		return
	}
	var code string
	var result <-chan paircode.Result
	for _, base := range bases {
		code, result, e = paircode.Listen(ctx, base, d.httpClient)
		if e == nil || ctx.Err() != nil {
			break
		}
	}
	if e != nil {
		d.deviceErrorFor(g)
		return
	}
	d.mu.Lock()
	if d.generation != g || d.started || ctx.Err() != nil {
		d.mu.Unlock()
		return
	}
	d.pairCode = code
	d.state = "将配对码发给协助人员，等待连接"
	d.mu.Unlock()
	select {
	case <-ctx.Done():
		return
	case r := <-result:
		if r.Err != nil {
			d.deviceErrorFor(g)
			return
		}
		d.launchCustomerFor(r.Invite, g)
	}
}
func (d *desktopServer) deviceError() {
	d.mu.Lock()
	g := d.generation
	d.mu.Unlock()
	d.deviceErrorFor(g)
}
func (d *desktopServer) deviceErrorFor(g uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started || d.generation != g {
		return
	}
	d.pairCode = ""
	d.listening = false
	d.phase = "error"
	d.state = "配对码已失效或生成失败，请检查网络后重新生成"
}

func (d *desktopServer) launchCustomer(invite string) bool { return d.launchCustomerFor(invite, 0) }
func (d *desktopServer) launchCustomerFor(invite string, expected uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started || (d.run == nil && d.start == nil) || (expected != 0 && expected != d.generation) {
		return false
	}
	if d.listenCancel != nil {
		d.listenCancel()
		d.listenCancel = nil
	}
	d.generation++
	g := d.generation
	ctx, cancel := context.WithTimeout(d.rootContext(), 2*time.Hour)
	done := make(chan struct{})
	d.sessionCancel = cancel
	d.sessionDone = done
	d.started = true
	d.listening = false
	d.pairCode = ""
	d.control = ""
	d.code = ""
	d.phase = "pairing"
	d.state = "正在核验协助人员"
	runner, legacy := d.run, d.start
	go func() {
		defer close(done)
		defer cancel()
		var err error
		if runner != nil {
			err = runner(ctx, invite, func(control, code string) {
				d.mu.Lock()
				defer d.mu.Unlock()
				if d.generation != g || !d.started || ctx.Err() != nil {
					return
				}
				d.control = control
				d.code = code
				d.phase = "awaiting_consent"
				d.state = "请核对身份并允许本次协助"
				if d.announce != nil {
					d.announce(control)
				}
			})
		} else {
			legacy(invite)
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.generation != g {
			return
		}
		d.started = false
		d.sessionCancel = nil
		d.control = ""
		d.code = ""
		d.phase = "ended"
		d.state = "本次协助已结束，可以生成新的配对码"
		if err != nil && ctx.Err() == nil {
			d.phase = "error"
			d.state = "连接未完成，请重新生成配对码后重试"
		}
	}()
	return true
}
func (d *desktopServer) resetCustomer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.listenCancel != nil {
		d.listenCancel()
		d.listenCancel = nil
	}
	d.listening = false
	d.pairCode = ""
	d.code = ""
	d.control = ""
	if d.started {
		d.phase = "ending"
		d.state = "正在结束本次协助，请稍候"
		if d.sessionCancel != nil {
			d.sessionCancel()
		}
		return
	}
	d.generation++
	d.phase = "idle"
	d.state = "可以生成新的配对码"
}
