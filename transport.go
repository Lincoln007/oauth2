// Copyright 2014 Google Inc. All Rights Reserved.
// Copyright 2014 The Macaron Authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package oauth2

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	defaultTokenType = "Bearer"
)

// Token represents the crendentials used to authorize
// the requests to access protected resources on the OAuth 2.0
// provider's backend.
type Token struct {
	// A token that authorizes and authenticates the requests.
	AccessToken string `json:"access_token"`

	// Identifies the type of token returned.
	TokenType string `json:"token_type,omitempty"`

	// A token that may be used to obtain a new access token.
	RefreshToken string `json:"refresh_token,omitempty"`

	// The remaining lifetime of the access token.
	Expiry time.Time `json:"expiry,omitempty"`

	// Raw optionally contains extra metadata from the server
	// when updating a token.
	Raw interface{}
}

// Extra returns an extra field returned from the server during token retrieval.
// E.g.
//     idToken := token.Extra("id_token")
//
func (t *Token) Extra(key string) string {
	if vals, ok := t.Raw.(url.Values); ok {
		return vals.Get(key)
	}
	if raw, ok := t.Raw.(map[string]interface{}); ok {
		if val, ok := raw[key].(string); ok {
			return val
		}
	}
	return ""
}

// Expired returns true if there is no access token or the
// access token is expired.
func (t *Token) Expired() bool {
	if t.AccessToken == "" {
		return true
	}
	if t.Expiry.IsZero() {
		return false
	}
	return t.Expiry.Before(time.Now())
}

// Transport is an http.RoundTripper that makes OAuth 2.0 HTTP requests.
type Transport struct {
	opts *Options
	base http.RoundTripper

	mu    sync.RWMutex
	token *Token
}

// NewTransport creates a new Transport that uses the provided
// token fetcher as token retrieving strategy. It authenticates
// the requests and delegates origTransport to make the actual requests.
func newTransport(base http.RoundTripper, opts *Options, token *Token) *Transport {
	return &Transport{
		base:  base,
		opts:  opts,
		token: token,
	}
}

// RoundTrip authorizes and authenticates the request with an
// access token. If no token exists or token is expired,
// tries to refresh/fetch a new token.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := t.token

	if token == nil || token.Expired() {
		// Check if the token is refreshable.
		// If token is refreshable, don't return an error,
		// rather refresh.
		if err := t.refreshToken(); err != nil {
			return nil, err
		}
		token = t.token
		if t.opts.TokenStore != nil {
			t.opts.TokenStore.WriteToken(token)
		}
	}

	// To set the Authorization header, we must make a copy of the Request
	// so that we don't modify the Request we were given.
	// This is required by the specification of http.RoundTripper.
	req = cloneRequest(req)
	typ := token.TokenType
	if typ == "" {
		typ = defaultTokenType
	}
	req.Header.Set("Authorization", typ+" "+token.AccessToken)
	return t.base.RoundTrip(req)
}

// Client returns an *http.Client that makes OAuth-authenticated requests.
func (t *Transport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// Token returns the token that authorizes and
// authenticates the transport.
func (t *Transport) Token() *Token {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.token
}

// refreshToken retrieves a new token, if a refreshing/fetching
// method is known and required credentials are presented
// (such as a refresh token).
func (t *Transport) refreshToken() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	token, err := t.opts.TokenFetcherFunc(t.token)
	if err != nil {
		return err
	}
	t.token = token
	return nil
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header)
	for k, s := range r.Header {
		r2.Header[k] = s
	}
	return r2
}
