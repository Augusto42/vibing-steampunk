package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// PackageMetadata contains the transport-relevant part of the direct ADT
// package resource. The package tree does not provide this information.
type PackageMetadata struct {
	Name              string
	RecordChanges     bool
	SoftwareComponent string
}

func (p PackageMetadata) RequiresTransport() bool {
	name := strings.ToUpper(strings.TrimSpace(p.Name))
	component := strings.ToUpper(strings.TrimSpace(p.SoftwareComponent))
	if strings.HasPrefix(name, "$") || component == "LOCAL" {
		return p.RecordChanges && !strings.HasPrefix(name, "$")
	}
	// Missing component metadata cannot prove that a named package is local.
	return true
}

func (c *Client) GetPackageMetadata(ctx context.Context, packageName string) (*PackageMetadata, error) {
	packageName = strings.ToUpper(strings.TrimSpace(packageName))
	if packageName == "" {
		return nil, fmt.Errorf("package name is required")
	}
	path := fmt.Sprintf("/sap/bc/adt/packages/%s", url.PathEscape(packageName))
	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.packages.v1+xml, application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("getting package %s metadata: %w", packageName, err)
	}
	type packageResource struct {
		Name       string `xml:"name,attr"`
		Attributes struct {
			RecordChanges bool `xml:"recordChanges,attr"`
		} `xml:"attributes"`
		Transport struct {
			SoftwareComponent struct {
				Name string `xml:"name,attr"`
			} `xml:"softwareComponent"`
		} `xml:"transport"`
	}
	var resource packageResource
	if err := xml.Unmarshal(resp.Body, &resource); err != nil {
		return nil, fmt.Errorf("parsing package %s metadata: %w", packageName, err)
	}
	if strings.TrimSpace(resource.Name) == "" {
		return nil, fmt.Errorf("parsing package %s metadata: response did not identify the package", packageName)
	}
	return &PackageMetadata{
		Name:              strings.ToUpper(resource.Name),
		RecordChanges:     resource.Attributes.RecordChanges,
		SoftwareComponent: strings.ToUpper(resource.Transport.SoftwareComponent.Name),
	}, nil
}

// checkPackageTransportRequirement runs before the first stateful mutation.
// It refuses to guess whether an unverified package is local or transportable.
func (c *Client) checkPackageTransportRequirement(ctx context.Context, packageName, transport, opName string) error {
	packageName = strings.ToUpper(strings.TrimSpace(packageName))
	transport = strings.ToUpper(strings.TrimSpace(transport))
	if packageName == "" || strings.HasPrefix(packageName, "$") {
		return nil
	}
	metadata, err := c.GetPackageMetadata(ctx, packageName)
	if err != nil {
		return fmt.Errorf("%s blocked before mutation: cannot determine whether package %s requires a transport: %w", opName, packageName, err)
	}
	if metadata.RequiresTransport() && transport == "" {
		return fmt.Errorf("%s blocked before mutation: package %s records changes and requires an explicit transport request", opName, packageName)
	}
	return nil
}
