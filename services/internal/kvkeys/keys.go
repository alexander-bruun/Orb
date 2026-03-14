// Package kvkeys defines the key schema for the KeyVal (Valkey) layer.
package kvkeys

import "strings"

func Session(userID string) string            { return "session:" + userID }
func RefreshToken(token string) string        { return "refresh:" + token }
func TrackMeta(trackID string) string         { return "track:meta:" + trackID }
func UserQueue(userID string) string          { return "queue:" + userID }
func LoginAttempts(ip string) string          { return "ratelimit:login:" + strings.ReplaceAll(ip, ":", "_") }
func ListenSession(id string) string          { return "listen_session:" + id }
func ListenGuestToken(token string) string    { return "listen_guest:" + token }
func TOTPPending(token string) string         { return "totp_pending:" + token }
func UserStreamingPrefs(userID string) string { return "user:stream_prefs:" + userID }
func ShareToken(token string) string          { return "share:" + token }
func ShareStreamSession(token string) string  { return "share_stream:" + token }

// Ingest coordination keys (distributed leader election, scan lock, work queue, SSE events).
func IngestLeader() string    { return "ingest:leader" }
func IngestScanLock() string  { return "ingest:scan_lock" }
func IngestWorkQueue() string { return "ingest:work_queue" }
func IngestEvents() string    { return "ingest:events" }

// Device session keys.
func UserDevice(userID, deviceID string) string { return "user:device:" + userID + ":" + deviceID }
func UserDeviceGlob(userID string) string       { return "user:device:" + userID + ":*" }
func UserActiveDevice(userID string) string     { return "user:active_device:" + userID }
func UserDeviceEvents(userID string) string     { return "user:device_events:" + userID }
func UserExclusiveMode(userID string) string    { return "user:exclusive_mode:" + userID }
