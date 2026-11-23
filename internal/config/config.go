// Package config holds nonsecret client preferences shared by desktop and CLI.
package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"pairshell/internal/securefile"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultServer is the unset custom preference.
const DefaultServer = ""

// OfficialServer is the default service when no custom server is selected.
// Release builds may override it.
var OfficialServer = "https://relay.pairshell.top"

const maxConfigBytes = 4096

type Settings struct {
	Server           string `json:"server"`
	ForceCustom      bool   `json:"force_custom,omitempty"`
	TrustedCAPEM     string `json:"trusted_ca_pem,omitempty"`
	ServerConfigured bool   `json:"server_configured,omitempty"`
}

func DefaultPath() (string, error) {
	base, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, "PairShell", "config.json"), nil
}
func ValidateServer(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t?#") {
		return "", errors.New("服务器地址必须是 HTTPS 根地址")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("服务器地址必须是 HTTPS 根地址，不含账号、路径、查询参数或片段")
	}
	if strings.ContainsAny(u.Hostname(), " \\/") {
		return "", errors.New("无效服务器主机")
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", errors.New("无效服务器端口")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return "", errors.New("无效服务器端口")
		}
	}
	u.Path = ""
	return u.String(), nil
}
func Load(path string) (Settings, error) {
	fallback := Settings{}
	s, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return fallback, nil
	}
	if e != nil {
		return Settings{}, e
	}
	if !s.Mode().IsRegular() || s.Size() > maxConfigBytes {
		return Settings{}, errors.New("配置不是有效的小型普通文件")
	}
	f, e := os.Open(path)
	if e != nil {
		return Settings{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if e != nil || len(b) > maxConfigBytes {
		return Settings{}, errors.New("配置读取失败或超过长度限制")
	}
	var cfg Settings
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&cfg); e != nil {
		return Settings{}, errors.New("配置格式无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Settings{}, errors.New("配置含多余内容")
	}
	legacyServer := strings.TrimSuffix(cfg.Server, "/")
	if !cfg.ServerConfigured && (legacyServer == "https://192.168.31.178:9443" || legacyServer == "https://192.168.31.178" || legacyServer == "https://167.104.165.174:9443" && !cfg.ForceCustom) {
		cfg.Server, cfg.TrustedCAPEM, cfg.ForceCustom = "", "", false
	}
	if cfg.Server != "" {
		cfg.Server, e = ValidateServer(cfg.Server)
	}
	if cfg.ForceCustom && cfg.Server == "" {
		return Settings{}, errors.New("强制自定义模式需要填写服务器地址")
	}
	return cfg, e
}
func LoadDefault() (Settings, error) {
	p, e := DefaultPath()
	if e != nil {
		return Settings{}, e
	}
	return Load(p)
}
func resolve(explicit, environment, path string) (string, error) {
	if explicit != "" {
		return ValidateServer(explicit)
	}
	if environment != "" {
		return ValidateServer(environment)
	}
	s, e := Load(path)
	if e != nil {
		return "", e
	}
	candidates, e := serverCandidates("", "", s, OfficialServer)
	if e != nil {
		return "", e
	}
	return candidates[0], nil
}

// ResolveServer applies flag > environment > saved preferences > official default.
func ResolveServer(explicit string) (string, error) {
	p, e := DefaultPath()
	if e != nil {
		return "", e
	}
	return resolve(explicit, os.Getenv("PAIRSHELL_SERVER"), p)
}
func Save(path string, cfg Settings) error {
	var e error
	if cfg.Server != "" {
		cfg.Server, e = ValidateServer(cfg.Server)
		if e != nil {
			return e
		}
	}
	if cfg.ForceCustom && cfg.Server == "" {
		return errors.New("强制自定义模式需要填写服务器地址")
	}
	cfg.ServerConfigured = cfg.Server != ""
	if s, e := os.Lstat(path); e == nil && !s.Mode().IsRegular() {
		return errors.New("配置目标不是普通文件")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	dir := filepath.Dir(path)
	if e = securefile.Mkdir(dir); e != nil {
		return e
	}
	b, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return e
	}
	tmp := filepath.Join(dir, ".config-"+hex.EncodeToString(nonce[:])+".tmp")
	if e = securefile.Write(tmp, b); e != nil {
		return e
	}
	defer os.Remove(tmp)
	return replace(tmp, path)
}
func SaveDefault(cfg Settings) error {
	p, e := DefaultPath()
	if e != nil {
		return e
	}
	return Save(p, cfg)
}
