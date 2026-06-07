// Package events is an SSE pub/sub hub. Producers anywhere in the app call
// Hub.Broadcast(type, data); every subscribed HTTP client receives the event as
// a JSON SSE frame. It mirrors Seanime's WebSocket hub shape (a manager holding
// clients + a string-const event-type registry + a Broadcast fan-out) but the
// transport is one-way SSE, so there is no inbound client->server half. Each
// client has a buffered channel and is dropped if it falls too far behind, so a
// single stalled browser tab can never block a producer goroutine.
package events

// Type is an SSE event-type discriminator. The frontend switches on it.
type Type string

// Event-type registry. Phases after this one emit these; the heartbeat is the
// hub's own keep-alive. Add a new event by adding one const here, never a new
// code path.
const (
	TypeDownloadProgress Type = "download.progress"
	TypeEncodeProgress   Type = "encode.progress"
	TypeEpisodeStatus    Type = "episode.status"
	TypeFeedChecked      Type = "feed.checked"
	TypeLog              Type = "log"
	TypeHeartbeat        Type = "heartbeat"
)

// Event is the wire payload: a type discriminator plus an arbitrary JSON body.
// It is serialized as an SSE frame: "event: <type>\ndata: <json(Data)>\n\n".
type Event struct {
	Type Type `json:"type"`
	Data any  `json:"data"`
}
