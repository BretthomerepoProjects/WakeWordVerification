package setup

import (
	"fmt"
	"html"
	"net/http"

	"github.com/HuskerMinion/techo5/echod/internal/feature/security"
)

func init() { routes["/setup/ssh"] = (*Feature).saveSSH }

func sshSection(w http.ResponseWriter, token string) {
	state := security.Get().State()
	if !state.SSHAvailable {
		return
	}
	status := "Disabled"
	switch {
	case state.SSH && state.SSHRunning:
		status = "Running"
	case state.SSH:
		status = "Waiting for SSH to start"
	case state.SSHRunning:
		status = "Stopping"
	}
	fmt.Fprintf(w, `<fieldset><legend>SSH</legend>
	 <p>Status: %s</p>
	 <form method="post" action="/setup/ssh">
	  <input type="hidden" name="token" value="%s">
	  <label>SSH public key <textarea name="public_key" rows="3" spellcheck="false"></textarea></label>
	  <div class="btns">
	   <button name="operation" value="enable">Save key and enable SSH</button>
	   <button name="operation" value="disable">Disable SSH</button>
	  </div>
	 </form></fieldset>`, status, html.EscapeString(token))
}

func (f *Feature) saveSSH(w http.ResponseWriter, r *http.Request) {
	token, in := f.session(r)
	if !in {
		http.Error(w, "not let in", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "post to change SSH", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := r.ParseForm(); err != nil || !sameToken(r.PostFormValue("token"), token) {
		http.Error(w, "that did not come from this page", http.StatusForbidden)
		return
	}
	f.used()
	switch r.PostFormValue("operation") {
	case "enable":
		if err := security.Get().EnableLocalSSH(r.PostFormValue("public_key")); err != nil {
			http.Redirect(w, r, back("privacy", "problem", "Could not enroll the SSH public key."), http.StatusSeeOther)
			return
		}
	case "disable":
		security.Get().SetSSH(false)
	default:
		http.Error(w, "unknown SSH operation", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/setup?tab=privacy", http.StatusSeeOther)
}
