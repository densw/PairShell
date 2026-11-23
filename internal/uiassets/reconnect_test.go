package uiassets

import("strings";"testing";"golang.org/x/net/html")

func TestReconnectActionsRemainInBothClientTemplates(t *testing.T){
 for _,tc:=range []struct{page string;ids []string}{{Merchant,[]string{"connect-form","pair-code","connect","merchant-error","reopen-terminal","end"}},{Invite,[]string{"regenerate-code","copy-code","state"}},{Control,[]string{"new-session","error"}}}{doc,e:=html.Parse(strings.NewReader(tc.page));if e!=nil{t.Fatal(e)};for _,id:=range tc.ids{if byID(doc,id)==nil{t.Errorf("missing actionable reconnect control %s",id)}}}
}
