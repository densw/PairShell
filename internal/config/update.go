package config

func updateServer(path, server string) error {
	return updatePreferences(path, server, false)
}
func updatePreferences(path, server string, force bool) error {
	next := server
	var e error
	if next != "" {
		next, e = ValidateServer(server)
		if e != nil {
			return e
		}
	}
	old, e := Load(path)
	if e != nil {
		return e
	}
	if old.Server != next {
		old.TrustedCAPEM = ""
	}
	old.Server = next
	old.ForceCustom = force
	return Save(path, old)
}
func UpdatePreferences(server string, force bool) error {
	path, e := DefaultPath()
	if e != nil {
		return e
	}
	return updatePreferences(path, server, force)
}
func UpdateServerDefault(server string) error {
	path, e := DefaultPath()
	if e != nil {
		return e
	}
	return updateServer(path, server)
}
