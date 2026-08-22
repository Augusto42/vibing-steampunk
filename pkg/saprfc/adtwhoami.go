package saprfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

// Under single sign-on nothing on this side knows who you are. The cookie
// carries a session, not a name; ADT's own session resource does not report
// one; and the configuration has no user because none was needed to log on.
// The debugger, though, cannot work without it — a listener is registered under
// a user name, and a listener registered under the wrong one waits out its
// whole timeout while the debuggee stops for nobody.
//
// The transport organizer answers it. Asking for your own requests returns a
// tree whose root is named after you, and it is named even when you own no
// requests at all — an empty result still carries the identity. That makes it a
// "who am I" over plain ADT: no RFC channel, no gateway, no Z code.
const whoAmIResource = "/sap/bc/adt/cts/transportrequests?_action=FIND&trfunction=K&trstatus=D&targetsystem="

// CurrentUser asks the system whose session this is.
func CurrentUser(ctx context.Context, transport ADTTransport) (string, error) {
	res, err := transport.Do(ctx, ADTRequest{
		Method: "GET",
		URI:    whoAmIResource,
		// Not a concrete type: this resource answers with its own
		// transportorganizertree media type, and naming application/xml here
		// gets a 406 on releases that take Accept literally — the same trap the
		// transport layer documents for every other ADT resource.
		Headers: []ADTHeader{{Name: "Accept", Value: "*/*"}},
	})
	if err != nil {
		return "", err
	}
	if res.Status != 200 {
		return "", adtError("who am I", res)
	}
	name := userFromTransportTree(res.Body)
	if name == "" {
		return "", fmt.Errorf("the transport organizer answered without naming a user")
	}
	return name, nil
}

// userFromTransportTree reads the owner out of a transport organizer tree.
func userFromTransportTree(body []byte) string {
	var root struct {
		Name      string `xml:"name,attr"`
		CreatedBy string `xml:"createdBy,attr"`
		ChangedBy string `xml:"changedBy,attr"`
	}
	if err := xml.Unmarshal(body, &root); err != nil {
		return ""
	}
	// The root is named after the owner; createdBy and changedBy say the same
	// thing, and are here because an empty tree on some releases carries only
	// those.
	for _, candidate := range []string{root.Name, root.CreatedBy, root.ChangedBy} {
		if name := strings.ToUpper(strings.TrimSpace(candidate)); name != "" {
			return name
		}
	}
	return ""
}
