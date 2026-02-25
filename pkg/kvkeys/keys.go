// Package kvkeys defines the key schema for the KeyVal (Valkey) layer.
package kvkeys

import "strings"

func Session(userID string) string     { return "session:" + userID }
func RefreshToken(token string) string { return "refresh:" + token }
func TrackMeta(trackID string) string  { return "track:meta:" + trackID }
func UserQueue(userID string) string   { return "queue:" + userID }
func LoginAttempts(ip string) string   { return "ratelimit:login:" + strings.ReplaceAll(ip, ":", "_") }
