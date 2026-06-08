// saml_metadata.go — IdP metadata XML parser.
//
// We avoid importing crewjam/saml/samlsp (which pulls golang-jwt/jwt/v4
// and an entire cookie-session middleware) by copying the ~20 lines
// of XML unmarshal we need. This is verbatim from samlsp/fetch_metadata.go;
// keeping it inline means the hub binary doesn't drag in samlsp's
// middleware machinery that we don't use.

package auth

import (
	"bytes"
	"encoding/xml"
	"errors"

	xrv "github.com/mattermost/xml-roundtrip-validator"

	"github.com/crewjam/saml"
)

// parseEntityDescriptor parses raw IdP metadata XML. The input may
// be wrapped in <EntitiesDescriptor> (common for IdP federations) or
// be a bare <EntityDescriptor>; both shapes are handled.
func parseEntityDescriptor(data []byte) (*saml.EntityDescriptor, error) {
	if err := xrv.Validate(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	entity := &saml.EntityDescriptor{}
	if err := xml.Unmarshal(data, entity); err != nil {
		// EntitiesDescriptor is the federation wrapper; unwrap to the
		// first EntityDescriptor that actually declares an IDPSSODescriptor.
		if err.Error() == "expected element type <EntityDescriptor> but have <EntitiesDescriptor>" {
			entities := &saml.EntitiesDescriptor{}
			if err := xml.Unmarshal(data, entities); err != nil {
				return nil, err
			}
			for i := range entities.EntityDescriptors {
				if len(entities.EntityDescriptors[i].IDPSSODescriptors) > 0 {
					return &entities.EntityDescriptors[i], nil
				}
			}
			return nil, errors.New("no entity found with IDPSSODescriptor")
		}
		return nil, err
	}
	return entity, nil
}
