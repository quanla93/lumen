// saml_marshal.go — XML marshal helper for the SP metadata endpoint.
//
// crewjam/saml's ServiceProvider.Metadata() returns a *saml.EntityDescriptor
// (a Go struct), not a byte slice. To produce the XML a real IdP
// ingests we marshal the struct. encoding/xml's behaviour with the
// saml.EntityDescriptor type works out of the box because the field
// tags are set; we use MarshalIndent for human-readable output.

package auth

import (
	"bytes"
	"encoding/xml"
	"errors"

	"github.com/crewjam/saml"
)

// marshalEntityDescriptor turns a *saml.EntityDescriptor into the
// canonical XML an IdP expects. We use the saml package's own
// schema types so the namespaces (md:, ds:, etc.) come out right
// without us hand-rolling them.
func marshalEntityDescriptor(ed *saml.EntityDescriptor) ([]byte, error) {
	if ed == nil {
		return nil, errors.New("SAML: nil EntityDescriptor")
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(ed); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
