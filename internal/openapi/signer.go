// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package openapi implements the minimal Volcengine OpenAPI client the CLI needs.
// Signing follows Volcengine's HMAC-SHA256 credential scope
// <date>/<region>/<service>/request and supports Signin-issued STS tokens.
package openapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	algorithm  = "HMAC-SHA256"
	timeFormat = "20060102T150405Z" // ISO8601 basic, UTC
)

type SignOptions struct {
	Service string
	Region  string
	Now     time.Time
	Body    []byte
}

func SignRequest(req *http.Request, credential Credential, options SignOptions) error {
	if strings.TrimSpace(credential.AccessKeyID) == "" {
		return fmt.Errorf("access key id is required")
	}
	if strings.TrimSpace(credential.SecretAccessKey) == "" {
		return fmt.Errorf("secret access key is required")
	}
	if strings.TrimSpace(options.Service) == "" {
		return fmt.Errorf("service is required")
	}
	if strings.TrimSpace(options.Region) == "" {
		return fmt.Errorf("region is required")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	options.Now = options.Now.UTC()
	if req.URL.Path == "" {
		req.URL.Path = "/"
	}
	host := req.URL.Host
	if req.Host != "" {
		host = req.Host
	}
	req.Host = host
	if req.Header.Get("Content-Type") == "" && req.Method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}

	xDate := options.Now.Format(timeFormat)
	bodyHash := hashSHA256(options.Body)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", bodyHash)
	if strings.TrimSpace(credential.SessionToken) != "" {
		req.Header.Set("X-Security-Token", credential.SessionToken)
	}

	date := xDate[:8]
	credentialScope := strings.Join([]string{date, options.Region, options.Service, "request"}, "/")
	headerMap := canonicalHeaderMap(req.Header, host)
	canonicalHeaders, signedHeaders := canonicalHeaders(headerMap)
	canonicalRequest := strings.Join([]string{
		req.Method,
		normURI(req.URL.Path),
		normQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")
	stringToSign := strings.Join([]string{
		algorithm,
		xDate,
		credentialScope,
		hashSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := signature(signingKey(credential.SecretAccessKey, date, options.Region, options.Service), stringToSign)
	req.Header.Set("Authorization", algorithm+" Credential="+credential.AccessKeyID+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return nil
}

func canonicalHeaderMap(header http.Header, host string) map[string]string {
	result := map[string]string{}
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		canonicalKey := strings.ToLower(key)
		switch key {
		case "Content-Type", "Content-Md5", "Host", "X-Security-Token":
		default:
			if !strings.HasPrefix(key, "X-") {
				continue
			}
		}
		result[canonicalKey] = strings.TrimSpace(values[0])
	}
	if strings.TrimSpace(host) != "" {
		result["host"] = normalizeHost(host)
	}
	return result
}

func canonicalHeaders(headers map[string]string) (string, string) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(':')
		builder.WriteString(strings.TrimSpace(headers[key]))
		builder.WriteByte('\n')
	}
	return builder.String(), strings.Join(keys, ";")
}

func normalizeHost(host string) string {
	if strings.Contains(host, ":") {
		split := strings.Split(host, ":")
		if len(split) == 2 && (split[1] == "80" || split[1] == "443") {
			return split[0]
		}
	}
	return host
}

func normURI(uri string) string {
	parts := strings.Split(uri, "/")
	for i := range parts {
		parts[i] = encodePathFrag(parts[i])
	}
	return strings.Join(parts, "/")
}

func encodePathFrag(value string) string {
	hexCount := 0
	for i := 0; i < len(value); i++ {
		if shouldEscape(value[i]) {
			hexCount++
		}
	}
	escaped := make([]byte, len(value)+2*hexCount)
	j := 0
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if shouldEscape(ch) {
			escaped[j] = '%'
			escaped[j+1] = "0123456789ABCDEF"[ch>>4]
			escaped[j+2] = "0123456789ABCDEF"[ch&15]
			j += 3
			continue
		}
		escaped[j] = ch
		j++
	}
	return string(escaped)
}

func shouldEscape(ch byte) bool {
	if 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' {
		return false
	}
	if '0' <= ch && ch <= '9' {
		return false
	}
	return ch != '-' && ch != '_' && ch != '.' && ch != '~'
}

func normQuery(values url.Values) string {
	return strings.ReplaceAll(values.Encode(), "+", "%20")
}

func signingKey(secretKey string, date string, region string, service string) []byte {
	kDate := hmacSHA256([]byte(secretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "request")
}

func signature(signingKey []byte, stringToSign string) string {
	return hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
}

func hmacSHA256(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(content))
	return mac.Sum(nil)
}

func hashSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
