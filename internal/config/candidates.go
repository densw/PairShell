package config

import (
	"errors"
	"os"
)

func serverCandidates(explicit, environment string, cfg Settings, official string) ([]string, error) {
	for _, value := range []string{explicit, environment} {
		if value != "" {
			v, e := ValidateServer(value)
			if e != nil {
				return nil, e
			}
			return []string{v}, nil
		}
	}
	out := []string{}
	if cfg.Server != "" {
		v, e := ValidateServer(cfg.Server)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if cfg.ForceCustom {
		if len(out) == 0 {
			return nil, errors.New("请先配置自定义服务器")
		}
		return out, nil
	}
	if official != "" {
		v, e := ValidateServer(official)
		if e != nil {
			return nil, e
		}
		if len(out) == 0 || out[0] != v {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("官方服务尚未开放，请在服务器配置中填写自定义服务器")
	}
	return out, nil
}

func ServerCandidates(explicit string) ([]string, error) {
	cfg, e := LoadDefault()
	if e != nil {
		return nil, e
	}
	return serverCandidates(explicit, os.Getenv("PAIRSHELL_SERVER"), cfg, OfficialServer)
}
