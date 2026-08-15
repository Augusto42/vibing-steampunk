// Package mocksap provides a deliberately small SAP ADT/ZADT_VSP protocol
// simulator for local integration tests. It is not an ABAP runtime and does
// not attempt to emulate SAP business behavior.
package mocksap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	Username = "SYNTHETIC_USER"
	Password = "synthetic-password"
	Client   = "001"

	PackageName     = "$TMP"
	IncludeName     = "ZSYNTHETIC_INCLUDE"
	EnhancementName = "ZSYNTHETIC_ENHO"
	ProgramName     = "ZSYNTHETIC_APP"
	ScreenNumber    = "0100"

	csrfToken  = "synthetic-csrf-token"
	lockHandle = "SYNTHETIC-LOCK-HANDLE"
)

const InitialIncludeSource = `INCLUDE zsynthetic_include.

FORM synthetic_form.
* $*$\SE:(1) Form SYNTHETIC_FORM, Start
  DATA lv_value TYPE i.
ENDFORM.`

const EnhancementSource = `ENHANCEMENT 1 zsynthetic_enho.
  lv_value = 42.
ENDENHANCEMENT.`

// Options controls deterministic fault injection. These switches are useful
// for proving that VSP fails closed before a write or on a bad RFC return code.
type Options struct {
	Username        string
	Password        string
	SyntaxError     bool
	ActivationError bool
	DynproSubrc     int
}

// RequestRecord is a sanitized trace of a request received by the simulator.
// Authorization values and request bodies are intentionally never retained.
type RequestRecord struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Query    string `json:"query,omitempty"`
	Stateful bool   `json:"stateful,omitempty"`
	CSRF     bool   `json:"csrf,omitempty"`
}

// State is the observable, synthetic-only state of the simulator.
type State struct {
	IncludeSource string          `json:"includeSource"`
	Requests      []RequestRecord `json:"requests"`
}

// Server implements the small subset of SAP protocols needed by the object
// expansion tests: ADT search/source/write endpoints and the ZADT_VSP RFC
// WebSocket used to read Dynpro metadata.
type Server struct {
	opts Options

	mu            sync.RWMutex
	includeSource string
	locked        bool
	requests      []RequestRecord

	upgrader websocket.Upgrader
}

// New returns a fresh, in-memory simulator with only synthetic fixtures.
func New(opts Options) *Server {
	if opts.Username == "" {
		opts.Username = Username
	}
	if opts.Password == "" {
		opts.Password = Password
	}
	return &Server{
		opts:          opts,
		includeSource: InitialIncludeSource,
		upgrader: websocket.Upgrader{
			CheckOrigin: sameHostOrNoOrigin,
		},
	}
}

func sameHostOrNoOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// Snapshot returns a copy of the current synthetic state.
func (s *Server) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return State{
		IncludeSource: s.includeSource,
		Requests:      append([]RequestRecord(nil), s.requests...),
	}
}

func (s *Server) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, RequestRecord{
		Method:   r.Method,
		Path:     r.URL.Path,
		Query:    r.URL.RawQuery,
		Stateful: strings.EqualFold(r.Header.Get("X-sap-adt-sessiontype"), "stateful"),
		CSRF:     r.Header.Get("X-CSRF-Token") == csrfToken,
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "kind": "synthetic-sap-protocol-simulator"})
		return
	}

	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="VSP synthetic SAP"`)
		http.Error(w, "synthetic credentials required", http.StatusUnauthorized)
		return
	}
	s.record(r)

	if r.URL.Path == "/sap/bc/apc/sap/zadt_vsp" {
		s.serveWebSocket(w, r)
		return
	}
	if r.URL.Path == "/mock/state" {
		writeJSON(w, http.StatusOK, s.Snapshot())
		return
	}
	if r.URL.Path == "/sap/bc/adt/core/discovery" {
		s.serveDiscovery(w, r)
		return
	}
	if isModifying(r.Method) && r.Header.Get("X-CSRF-Token") != csrfToken {
		w.Header().Set("X-CSRF-Token", "Required")
		http.Error(w, "synthetic CSRF token required", http.StatusForbidden)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/sap/bc/adt/repository/informationsystem/search":
		s.serveSearch(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/sap/bc/adt/enhancements/enhoxhs":
		s.serveEnhancementReferences(w)
	case r.Method == http.MethodGet && isEnhancementSourcePath(r.URL.Path):
		writeText(w, http.StatusOK, EnhancementSource)
	case r.URL.Path == "/sap/bc/adt/programs/includes/"+IncludeName+"/source/main":
		s.serveIncludeSource(w, r)
	case r.URL.Path == "/sap/bc/adt/programs/includes/"+IncludeName:
		s.serveIncludeLock(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/checkruns":
		s.serveSyntaxCheck(w)
	case r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/activation":
		s.serveActivation(w)
	default:
		http.Error(w, "synthetic endpoint not implemented", http.StatusNotFound)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	username, password, ok := r.BasicAuth()
	return ok && username == s.opts.Username && password == s.opts.Password
}

func (s *Server) serveDiscovery(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Header.Get("X-CSRF-Token"), "fetch") {
		w.Header().Set("X-CSRF-Token", csrfToken)
		http.SetCookie(w, &http.Cookie{
			Name:     "sap-contextid",
			Value:    "SYNTHETIC-SESSION",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	w.Header().Set("Content-Type", "application/atomsvc+xml")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><service xmlns="http://www.w3.org/2007/app"/>`))
	}
}

func (s *Server) serveSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("query")))
	refs := make([]string, 0, 2)
	if query == "" || query == EnhancementName || wildcardMatch(query, EnhancementName) {
		refs = append(refs, fmt.Sprintf(`<adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/%s" adtcore:type="ENHO/XH" adtcore:name="%s" adtcore:packageName="%s" adtcore:description="Synthetic enhancement"/>`, strings.ToLower(EnhancementName), EnhancementName, PackageName))
	}
	if query == "" || query == IncludeName || wildcardMatch(query, IncludeName) {
		refs = append(refs, fmt.Sprintf(`<adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/includes/%s" adtcore:type="PROG/I" adtcore:name="%s" adtcore:packageName="%s" adtcore:description="Synthetic include"/>`, IncludeName, IncludeName, PackageName))
	}
	body := `<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">` + strings.Join(refs, "") + `</adtcore:objectReferences>`
	writeXML(w, http.StatusOK, body)
}

func wildcardMatch(pattern, value string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	position := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		index := strings.Index(value[position:], part)
		if index < 0 {
			return false
		}
		position += index + len(part)
	}
	return true
}

func (s *Server) serveEnhancementReferences(w http.ResponseWriter) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/%s" adtcore:type="ENHO/XH" adtcore:name="%s" adtcore:packageName="%s" adtcore:description="Synthetic enhancement"/></adtcore:objectReferences>`, strings.ToLower(EnhancementName), EnhancementName, PackageName)
	writeXML(w, http.StatusOK, body)
}

func isEnhancementSourcePath(path string) bool {
	name := strings.ToLower(EnhancementName)
	return path == "/sap/bc/adt/enhancements/enhoxh/"+name+"/source/main" ||
		path == "/sap/bc/adt/enhancements/enhoxhs/"+name+"/source/main"
}

func (s *Server) serveIncludeSource(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		source := s.includeSource
		s.mu.RUnlock()
		writeText(w, http.StatusOK, source)
	case http.MethodPut:
		if !s.isLocked() || r.URL.Query().Get("lockHandle") != lockHandle {
			http.Error(w, "synthetic lock required", http.StatusLocked)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "reading synthetic source", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.includeSource = string(body)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) serveIncludeLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch strings.ToUpper(r.URL.Query().Get("_action")) {
	case "LOCK":
		if !strings.EqualFold(r.Header.Get("X-sap-adt-sessiontype"), "stateful") {
			http.Error(w, "stateful ADT session required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		if s.locked {
			s.mu.Unlock()
			http.Error(w, "synthetic object already locked", http.StatusLocked)
			return
		}
		s.locked = true
		s.mu.Unlock()
		writeXML(w, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><LOCK_HANDLE>`+lockHandle+`</LOCK_HANDLE></DATA></asx:values></asx:abap>`)
	case "UNLOCK":
		if r.URL.Query().Get("lockHandle") != lockHandle {
			http.Error(w, "invalid synthetic lock", http.StatusLocked)
			return
		}
		s.mu.Lock()
		s.locked = false
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "unknown synthetic lock action", http.StatusBadRequest)
	}
}

func (s *Server) isLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked
}

func (s *Server) serveSyntaxCheck(w http.ResponseWriter) {
	if !s.opts.SyntaxError {
		writeXML(w, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"/>`)
		return
	}
	body := `<?xml version="1.0" encoding="UTF-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"><chkrun:checkReport><chkrun:checkMessageList><chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/includes/ZSYNTHETIC_INCLUDE/source/main#start=2,1" chkrun:type="E" chkrun:shortText="Synthetic syntax error"/></chkrun:checkMessageList></chkrun:checkReport></chkrun:checkRunReports>`
	writeXML(w, http.StatusOK, body)
}

func (s *Server) serveActivation(w http.ResponseWriter) {
	if !s.opts.ActivationError {
		w.WriteHeader(http.StatusOK)
		return
	}
	body := `<activation><messages><msg type="E"><shortText><txt>Synthetic activation error</txt></shortText></msg></messages></activation>`
	writeXML(w, http.StatusOK, body)
}

func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"id":      "welcome",
		"success": true,
		"data": map[string]any{
			"session": "SYNTHETIC-WS-SESSION",
			"version": "mock-1.0",
			"domains": []string{"rfc"},
		},
	}); err != nil {
		return
	}

	for {
		var message struct {
			ID     string         `json:"id"`
			Domain string         `json:"domain"`
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		if message.Domain != "rfc" {
			_ = writeWSError(conn, message.ID, "UNSUPPORTED_DOMAIN", "synthetic server only implements the rfc domain")
			continue
		}

		switch message.Action {
		case "call":
			function, _ := message.Params["function"].(string)
			if !strings.EqualFold(function, "RPY_DYNPRO_READ") {
				_ = writeWSError(conn, message.ID, "UNSUPPORTED_RFC", "synthetic server only implements RPY_DYNPRO_READ")
				continue
			}
			program, _ := message.Params["PROGNAME"].(string)
			screen, _ := message.Params["DYNNR"].(string)
			if !strings.EqualFold(program, ProgramName) || screen != ScreenNumber {
				_ = writeWSError(conn, message.ID, "DYNP_NOT_FOUND", "synthetic dynpro not found")
				continue
			}
			_ = conn.WriteJSON(map[string]any{
				"id":      message.ID,
				"success": true,
				"data": map[string]any{
					"subrc": s.opts.DynproSubrc,
					"exports": map[string]any{
						"HEADER": map[string]any{"PROGRAM": ProgramName, "SCREEN": ScreenNumber, "TYPE": "N", "DESCRIPTION": "Synthetic main screen"},
					},
					"tables": map[string]any{
						"CONTAINERS":           []any{map[string]any{"NAME": "SYNTHETIC_MAIN", "TYPE": "NORMAL"}},
						"FIELDS_TO_CONTAINERS": []any{map[string]any{"NAME": "SYNTHETIC_VALUE", "CONTAINER": "SYNTHETIC_MAIN"}},
						"FLOW_LOGIC": []any{
							map[string]any{"LINE": "PROCESS BEFORE OUTPUT."},
							map[string]any{"LINE": "  MODULE SYNTHETIC_STATUS_0100."},
							map[string]any{"LINE": "PROCESS AFTER INPUT."},
						},
					},
				},
			})
		case "readSource":
			program, _ := message.Params["program"].(string)
			if !strings.EqualFold(program, EnhancementName+"E") {
				_ = writeWSError(conn, message.ID, "SOURCE_NOT_FOUND", "synthetic source not found")
				continue
			}
			_ = conn.WriteJSON(map[string]any{
				"id":      message.ID,
				"success": true,
				"data": map[string]any{
					"program": EnhancementName + "E",
					"source":  strings.Split(EnhancementSource, "\n"),
				},
			})
		default:
			_ = writeWSError(conn, message.ID, "UNSUPPORTED_ACTION", "synthetic RFC action not implemented")
		}
	}
}

func writeWSError(conn *websocket.Conn, id, code, message string) error {
	return conn.WriteJSON(map[string]any{
		"id":      id,
		"success": false,
		"error":   map[string]string{"code": code, "message": message},
	})
}

func isModifying(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func writeXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
