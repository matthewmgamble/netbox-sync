package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State represents the synchronization state
type State struct {
	LastSync        time.Time              `json:"last_sync"`
	SyncID          string                 `json:"sync_id"`
	DurationSeconds float64                `json:"duration_seconds"`
	Status          string                 `json:"status"` // "running", "completed", "failed"
	Objects         map[string]ObjectStats `json:"objects"`
	APICalls        APICallStats           `json:"api_calls"`
	Errors          []ErrorRecord          `json:"errors"`
	TrackedObjects  map[string]ObjectMap   `json:"tracked_objects"`
}

// ObjectStats represents statistics for a type of object
type ObjectStats struct {
	Discovered int `json:"discovered"`
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Deleted    int `json:"deleted"`
	Unchanged  int `json:"unchanged"`
	Errors     int `json:"errors"`
}

// APICallStats tracks API call counts
type APICallStats struct {
	Mist   int `json:"mist"`
	NetBox int `json:"netbox"`
}

// ErrorRecord represents an error that occurred during sync
type ErrorRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Object    string    `json:"object"`
	ObjectID  string    `json:"object_id"`
	Operation string    `json:"operation"`
	Error     string    `json:"error"`
}

// ObjectMap maps source IDs to NetBox IDs
type ObjectMap map[string]ObjectRecord

// ObjectRecord represents a tracked object
type ObjectRecord struct {
	NetBoxID    int64     `json:"netbox_id"`
	LastUpdated time.Time `json:"last_updated"`
	Checksum    string    `json:"checksum"`
}

// Manager manages state persistence
type Manager struct {
	filePath string
	state    *State
}

// NewManager creates a new state manager
func NewManager(filePath string) *Manager {
	return &Manager{
		filePath: filePath,
		state:    newState(),
	}
}

// newState creates a new empty state
func newState() *State {
	return &State{
		Status:         "initialized",
		Objects:        make(map[string]ObjectStats),
		TrackedObjects: make(map[string]ObjectMap),
		Errors:         []ErrorRecord{},
	}
}

// Load loads the state from disk
func (m *Manager) Load() error {
	// If file doesn't exist, start with empty state
	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		m.state = newState()
		return nil
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	m.state = &state
	return nil
}

// Save saves the state to disk
func (m *Manager) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temporary file first, then rename (atomic operation)
	tmpFile := m.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpFile, m.filePath); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// Get returns the current state
func (m *Manager) Get() *State {
	return m.state
}

// StartSync marks the beginning of a sync operation
func (m *Manager) StartSync(syncID string) {
	m.state = newState()
	m.state.SyncID = syncID
	m.state.LastSync = time.Now()
	m.state.Status = "running"
}

// EndSync marks the end of a sync operation
func (m *Manager) EndSync(success bool) {
	duration := time.Since(m.state.LastSync).Seconds()
	m.state.DurationSeconds = duration

	if success {
		m.state.Status = "completed"
	} else {
		m.state.Status = "failed"
	}
}

// RecordObjectStats records statistics for an object type
func (m *Manager) RecordObjectStats(objectType string, stats ObjectStats) {
	m.state.Objects[objectType] = stats
}

// RecordAPICall records an API call
func (m *Manager) RecordAPICall(service string) {
	switch service {
	case "mist":
		m.state.APICalls.Mist++
	case "netbox":
		m.state.APICalls.NetBox++
	}
}

// RecordError records an error
func (m *Manager) RecordError(object, objectID, operation string, err error) {
	m.state.Errors = append(m.state.Errors, ErrorRecord{
		Timestamp: time.Now(),
		Object:    object,
		ObjectID:  objectID,
		Operation: operation,
		Error:     err.Error(),
	})
}

// TrackObject records the mapping between source and NetBox IDs
func (m *Manager) TrackObject(objectType, sourceID string, netboxID int64, checksum string) {
	if m.state.TrackedObjects[objectType] == nil {
		m.state.TrackedObjects[objectType] = make(ObjectMap)
	}

	m.state.TrackedObjects[objectType][sourceID] = ObjectRecord{
		NetBoxID:    netboxID,
		LastUpdated: time.Now(),
		Checksum:    checksum,
	}
}

// GetTrackedObject retrieves a tracked object
func (m *Manager) GetTrackedObject(objectType, sourceID string) (ObjectRecord, bool) {
	if m.state.TrackedObjects[objectType] == nil {
		return ObjectRecord{}, false
	}

	record, exists := m.state.TrackedObjects[objectType][sourceID]
	return record, exists
}

// IsChanged checks if an object has changed based on checksum
func (m *Manager) IsChanged(objectType, sourceID, checksum string) bool {
	record, exists := m.GetTrackedObject(objectType, sourceID)
	if !exists {
		return true // New object
	}
	return record.Checksum != checksum
}

// GetSummary returns a human-readable summary of the sync
func (m *Manager) GetSummary() string {
	summary := fmt.Sprintf("Sync %s - Status: %s\n", m.state.SyncID, m.state.Status)
	summary += fmt.Sprintf("Duration: %.1fs\n\n", m.state.DurationSeconds)

	for objType, stats := range m.state.Objects {
		summary += fmt.Sprintf("%s:\n", objType)
		summary += fmt.Sprintf("  Discovered: %d\n", stats.Discovered)
		summary += fmt.Sprintf("  Created: %d\n", stats.Created)
		summary += fmt.Sprintf("  Updated: %d\n", stats.Updated)
		summary += fmt.Sprintf("  Deleted: %d\n", stats.Deleted)
		summary += fmt.Sprintf("  Unchanged: %d\n", stats.Unchanged)
		if stats.Errors > 0 {
			summary += fmt.Sprintf("  Errors: %d\n", stats.Errors)
		}
		summary += "\n"
	}

	summary += fmt.Sprintf("API Calls:\n")
	summary += fmt.Sprintf("  Mist: %d\n", m.state.APICalls.Mist)
	summary += fmt.Sprintf("  NetBox: %d\n", m.state.APICalls.NetBox)

	if len(m.state.Errors) > 0 {
		summary += fmt.Sprintf("\nErrors: %d\n", len(m.state.Errors))
		for i, err := range m.state.Errors {
			if i >= 10 {
				summary += fmt.Sprintf("  ... and %d more\n", len(m.state.Errors)-10)
				break
			}
			summary += fmt.Sprintf("  [%s] %s/%s: %s\n", err.Operation, err.Object, err.ObjectID, err.Error)
		}
	}

	return summary
}
