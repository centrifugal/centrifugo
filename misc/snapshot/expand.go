package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// defaultJWTSecret matches client.token.hmac_secret_key in the base config.
const defaultJWTSecret = "snapshot-secret"

// expandJSON replaces special values in scripted JSON:
//   - "$now" / "$now+N" / "$now-N": unix seconds as a number;
//   - {"$jwt": {claims}, "secret": "..."}: an HS256 token string;
//   - {"$b64": "text"}: base64 of text (for proto bytes fields).
func expandJSON(raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	v, err := DecodeJSON(raw)
	if err != nil {
		return nil, err
	}
	v, err = expandValue(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func expandValue(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if claims, ok := x["$jwt"]; ok {
			secret := defaultJWTSecret
			if s, ok := x["secret"].(string); ok {
				secret = s
			}
			c, err := expandValue(claims)
			if err != nil {
				return nil, err
			}
			return signHS256(c, secret)
		}
		if s, ok := x["$b64"].(string); ok && len(x) == 1 {
			return base64.StdEncoding.EncodeToString([]byte(s)), nil
		}
		for k, val := range x {
			e, err := expandValue(val)
			if err != nil {
				return nil, err
			}
			x[k] = e
		}
		return x, nil
	case []any:
		for i := range x {
			e, err := expandValue(x[i])
			if err != nil {
				return nil, err
			}
			x[i] = e
		}
		return x, nil
	case string:
		if strings.HasPrefix(x, "$now") {
			rest := strings.TrimPrefix(x, "$now")
			off := int64(0)
			if rest != "" {
				n, err := strconv.ParseInt(rest, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("bad %q: %w", x, err)
				}
				off = n
			}
			return json.Number(strconv.FormatInt(time.Now().Unix()+off, 10)), nil
		}
		return x, nil
	default:
		return v, nil
	}
}

func signHS256(claims any, secret string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
