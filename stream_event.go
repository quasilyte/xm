package xm

import (
	"math"
)

// StreamEventKind is an event tag that should be used to differentiate between different event types.
// See StreamEvent docs for more info.
//
// Experimental: the events handling API may change significantly in the future.
type StreamEventKind int

const (
	// EventUnknown is a sentinel value.
	// You should never receive an event of this kind.
	EventUnknown StreamEventKind = iota

	// EventNote is emitted every time a channel starts to play some note.
	// It can be triggered even of a "ghost note", so it's up to the application
	// to decide whether they need to handle that note or not.
	//
	// Use StreamEvent.NoteEventData to get the event data.
	//
	// Experimental: the events handling API may change significantly in the future.
	EventNote

	// EventSync tells the application to update their time counter to the specified value.
	//
	// As any other event, the sync event has a Time field that you should use as a
	// description of when the counter should be updated.
	// Therefore, a sync event with Time=2.0 and data argument of 2.5 should
	// force the application to set its time counter to 2.5, but only if
	// it already reached a time counter value of 2.0.
	//
	// Use StreamEvent.EventSyncData to get the event data.
	//
	// Experimental: the events handling API may change significantly in the future.
	EventSync
)

// StreamEvent holds a single Stream event data.
// This object is an argument to the Stream.SetEventHandler function.
//
// To handle the event correctly, you must first check its kind.
// For an event of kind EventNote there is a NoteEventData method that
// will return the associated data. For EventSync there is a SyncEventData.
//
// Every event has a Time value. This is a moment when this event happened in
// relation to the XM track start (in seconds). The user application needs
// to calculate the time deltas on its own and handle these events in the right moment.
//
// Experimental: the events handling API may change significantly in the future.
type StreamEvent struct {
	Kind StreamEventKind

	// Channel is an event channel ID.
	// They may not match the exact XM module channel IDs,
	// but different channel IDs are guaranteed to have unique IDs.
	//
	// Some events may be channel-independent.
	Channel int

	// Time represents the playback offset in seconds.
	// Time=2.5 means that this event happened somewhere around 2.5 seconds.
	Time float64

	value uint64
}

// NoteEventData returns the event data if e.Kind=EventSync.
// The return values are: note, instrument (id), volume.
// If there is no instrument, -1 is returned.
func (e StreamEvent) NoteEventData() (note, instrument int, vol float32) {
	noteBits := e.value & 0xff
	instrumentBits := (e.value >> 8) & 0xff
	volBits := e.value >> 16
	instrumentID := int(instrumentBits)
	if instrumentID == 255 {
		instrumentID = -1
	}
	return int(noteBits), instrumentID, math.Float32frombits(uint32(volBits))
}

// SyncEventData returns the event data if e.Kind=EventNote.
// The return values are: a time to synchronize to.
func (e StreamEvent) SyncEventData() (t float64) {
	return math.Float64frombits(e.value)
}
