package uiassets

import (
	"golang.org/x/net/html"
	"strings"
	"testing"
)

func byID(n *html.Node, id string) *html.Node {
	for _, a := range n.Attr {
		if a.Key == "id" && a.Val == id {
			return n
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := byID(c, id); found != nil {
			return found
		}
	}
	return nil
}
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
func TestCustomerCodeIsPrimaryAndSettingsAreOptional(t *testing.T) {
	doc, e := html.Parse(strings.NewReader(Invite))
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"pair-code", "copy-code", "server", "save-server", "settings-error", "invite", "start", "merchant-mode", "state", "code", "control"} {
		if byID(doc, id) == nil {
			t.Errorf("missing customer interface contract %q", id)
		}
	}
	legacy := byID(doc, "invite")
	if legacy == nil {
		return
	}
	insideDetails := false
	for p := legacy.Parent; p != nil; p = p.Parent {
		if p.Data == "details" {
			insideDetails = true
			if hasAttr(p, "open") {
				t.Fatal("advanced invitation expanded by default")
			}
			break
		}
	}
	if !insideDetails {
		t.Fatal("legacy invite must be an optional advanced flow")
	}
}
func TestCompleteAssistanceDefaultsAreVisibleWithoutPermissionDialog(t *testing.T) {
	doc, e := html.Parse(strings.NewReader(Control))
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"terminal", "tasks", "file_read", "file_write", "ai", "verification", "status", "outputs", "events", "error"} {
		n := byID(doc, id)
		if n == nil {
			t.Errorf("missing control interface contract %q", id)
			continue
		}
		if id == "terminal" || id == "tasks" || id == "file_read" || id == "file_write" || id == "ai" {
			if !hasAttr(n, "checked") {
				t.Errorf("complete assistance capability %q should be selected", id)
			}
			for p := n.Parent; p != nil; p = p.Parent {
				if p.Data == "dialog" || p.Data == "details" {
					t.Errorf("capability %q hidden behind extra steps", id)
				}
			}
		}
	}
	if byID(doc, "permissions-dialog") != nil || byID(doc, "file-root") != nil {
		t.Fatal("complete assistance still asks for a shared directory or permission dialog")
	}
}
