package namecheap

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/turushan/cheep/internal/provider"
)

// CallAPI executes one method from Cheep's official Namecheap catalog. Read
// calls use the normal retry policy. Mutations use the single-attempt writer so
// an ambiguous request is never sent a second time.
func (c *Client) CallAPI(ctx context.Context, call provider.APICall) (provider.APIResponse, error) {
	method := strings.TrimSpace(call.Method)
	if method == "" {
		return provider.APIResponse{}, &provider.Error{Kind: provider.ErrorInvalid, Message: "API method is required"}
	}

	body := make(map[string]string, len(call.Params)+1)
	for key, value := range call.Params {
		body[key] = value
	}
	body["Command"] = method

	client := c.sdk
	if call.Mutation {
		client = c.writer
	}
	var root genericXMLNode
	if _, err := client.DoXMLWithContext(ctx, body, &root); err != nil {
		converted := c.convertError(err)
		converted = redactGenericCallError(converted, call.Params)
		if call.Mutation && !isDefiniteMutationFailure(err) {
			return provider.APIResponse{}, &provider.Error{
				Kind:    provider.ErrorOutcomeUnknown,
				Message: fmt.Sprintf("Namecheap did not return a definite result for %s: %s", method, converted.Error()),
				Cause:   err,
			}
		}
		return provider.APIResponse{}, converted
	}

	commandResponse := root.child("CommandResponse")
	if commandResponse == nil {
		return provider.APIResponse{}, responseError("API call returned no CommandResponse")
	}
	result := provider.APIResponse{
		Method:           method,
		Status:           root.attribute("Status"),
		RequestedCommand: root.childText("RequestedCommand"),
		Response:         commandResponse.providerElement(),
		ExecutionTime:    root.childText("ExecutionTime"),
	}
	if warnings := root.child("Warnings"); warnings != nil {
		for i := range warnings.Children {
			warning := warnings.Children[i]
			if !strings.EqualFold(warning.Name, "Warning") {
				continue
			}
			result.Warnings = append(result.Warnings, provider.APIMessage{
				Number:  warning.attribute("Number"),
				Message: strings.TrimSpace(warning.Text),
			})
		}
	}
	return result, nil
}

type genericXMLNode struct {
	Name       string
	Attributes []xml.Attr
	Text       string
	Children   []genericXMLNode
}

func (n *genericXMLNode) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	n.Name = start.Name.Local
	n.Attributes = append([]xml.Attr(nil), start.Attr...)
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			var child genericXMLNode
			if err := decoder.DecodeElement(&child, &value); err != nil {
				return err
			}
			n.Children = append(n.Children, child)
		case xml.CharData:
			text.Write([]byte(value))
		case xml.EndElement:
			if value.Name == start.Name {
				n.Text = strings.TrimSpace(text.String())
				return nil
			}
		}
	}
}

func (n *genericXMLNode) child(name string) *genericXMLNode {
	for i := range n.Children {
		if strings.EqualFold(n.Children[i].Name, name) {
			return &n.Children[i]
		}
	}
	return nil
}

func (n *genericXMLNode) childText(name string) string {
	child := n.child(name)
	if child == nil {
		return ""
	}
	return child.Text
}

func (n *genericXMLNode) attribute(name string) string {
	for _, attribute := range n.Attributes {
		if strings.EqualFold(attribute.Name.Local, name) {
			return attribute.Value
		}
	}
	return ""
}

func (n genericXMLNode) providerElement() provider.XMLElement {
	element := provider.XMLElement{Name: n.Name, Text: n.Text}
	if len(n.Attributes) > 0 {
		element.Attributes = make(map[string]string, len(n.Attributes))
		for _, attribute := range n.Attributes {
			element.Attributes[attribute.Name.Local] = attribute.Value
		}
	}
	if len(n.Children) > 0 {
		element.Children = make([]provider.XMLElement, 0, len(n.Children))
		for _, child := range n.Children {
			element.Children = append(element.Children, child.providerElement())
		}
	}
	return element
}

func redactGenericCallError(err error, params map[string]string) error {
	providerError, ok := err.(*provider.Error)
	if !ok {
		return err
	}
	message := providerError.Message
	for key, value := range params {
		if value == "" || !sensitiveAPIParameter(key) {
			continue
		}
		message = strings.ReplaceAll(message, value, "[REDACTED]")
	}
	if message == providerError.Message {
		return err
	}
	copy := *providerError
	copy.Message = message
	return &copy
}

func sensitiveAPIParameter(name string) bool {
	normalized := strings.ToLower(name)
	for _, part := range []string{"password", "eppcode", "authorizationcode", "csr", "cardnumber", "cvv", "securitycode"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}
