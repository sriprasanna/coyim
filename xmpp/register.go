// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package xmpp implements the XMPP IM protocol, as specified in RFC 6120 and
// 6121.
package xmpp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

var (
	ErrUsernameConlict                 = errors.New("xmpp: the username is not available for registration")
	ErrMissingRequiredRegistrationInfo = errors.New("xmpp: missing required registration information")
	ErrRegistrationFailed              = errors.New("xmpp: account creation failed")
)

type inBandRegistration struct {
	XMLName xml.Name `xml:"http://jabber.org/features/iq-register register,omitempty"`
}

// RegisterQuery contains register query information for creating a new account
type RegisterQuery struct {
	XMLName  xml.Name  `xml:"jabber:iq:register query"`
	Username *xml.Name `xml:"username"`
	Password *xml.Name `xml:"password"`
	Form     Form      `xml:"x"`
	Datas    []bobData `xml:"data"`
}

// XEP-0077
func (d *Dialer) negotiateInBandRegistration(c *Conn) error {
	if c.features.InBandRegistration == nil {
		return nil
	}

	user := d.getJIDLocalpart()
	password := d.Password
	return c.createAccount(user, password)
}

func (c *Conn) createAccount(user, password string) error {
	if c.config.CreateCallback == nil {
		return nil
	}

	io.WriteString(c.config.getLog(), "Attempting to create account\n")
	fmt.Fprintf(c.out, "<iq type='get' id='create_1'><query xmlns='jabber:iq:register'/></iq>")
	var iq ClientIQ
	if err := c.in.DecodeElement(&iq, nil); err != nil {
		return errors.New("unmarshal <iq>: " + err.Error())
	}

	if iq.Type != "result" {
		return errors.New("xmpp: account creation failed")
	}
	var register RegisterQuery
	if err := xml.NewDecoder(bytes.NewBuffer(iq.Query)).Decode(&register); err != nil {
		return err
	}

	if len(register.Form.Type) > 0 {
		reply, err := processForm(&register.Form, register.Datas, c.config.CreateCallback)
		fmt.Fprintf(c.rawOut, "<iq type='set' id='create_2'><query xmlns='jabber:iq:register'>")
		if err = xml.NewEncoder(c.rawOut).Encode(reply); err != nil {
			return err
		}

		fmt.Fprintf(c.rawOut, "</query></iq>")
	} else if register.Username != nil && register.Password != nil {
		//TODO: make sure this only happens via SSL
		//TODO: should generate form asking for username and password,
		//and call processForm for consistency

		// Try the old-style registration.
		fmt.Fprintf(c.rawOut, "<iq type='set' id='create_2'><query xmlns='jabber:iq:register'><username>%s</username><password>%s</password></query></iq>", user, password)
	}

	var iq2 ClientIQ
	if err := c.in.DecodeElement(&iq2, nil); err != nil {
		return errors.New("unmarshal <iq>: " + err.Error())
	}

	if iq2.Type == "error" {
		switch iq2.Error.Code {
		case "409":
			// <conflict xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>
			return ErrUsernameConlict
		case "406":
			// <not-acceptable xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>
			return ErrMissingRequiredRegistrationInfo
		default:
			return ErrRegistrationFailed
		}
	}

	return nil
}

// CancelRegistration cancels the account registration with the server
func (c *Conn) CancelRegistration() (reply chan Stanza, cookie Cookie, err error) {
	// https://xmpp.org/extensions/xep-0077.html#usecases-cancel
	registrationCancel := rawXML(`
	<query xmlns='jabber:iq:register'>
		<remove/>
	</query>
	`)

	return c.SendIQ("", "set", registrationCancel)
}
